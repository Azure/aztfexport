package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	golog "log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/aztfy/internal/cfgfile"
	internalconfig "github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/pkg/telemetry"
	"github.com/gofrs/uuid"
	"github.com/pkg/profile"

	"github.com/Azure/aztfy/pkg/config"
	"github.com/Azure/aztfy/pkg/log"

	"github.com/hashicorp/go-hclog"
	"github.com/magodo/armid"
	"github.com/magodo/azlist/azlist"
	"github.com/magodo/tfadd/providers/azurerm"

	"github.com/Azure/aztfy/internal"
	"github.com/Azure/aztfy/internal/ui"
	"github.com/Azure/aztfy/internal/utils"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	azlog "github.com/Azure/azure-sdk-for-go/sdk/azcore/log"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/urfave/cli/v2"
)

var (
	flagLogPath  string
	flagLogLevel string
)

func main() {
	var (
		// common flags
		flagSubscriptionId      string
		flagOutputDir           string
		flagOverwrite           bool
		flagAppend              bool
		flagDevProvider         bool
		flagBackendType         string
		flagBackendConfig       cli.StringSlice
		flagFullConfig          bool
		flagParallelism         int
		flagContinue            bool
		flagNonInteractive      bool
		flagGenerateMappingFile bool
		flagHCLOnly             bool
		flagModulePath          string

		// common flags (hidden)
		hflagMockClient bool
		hflagPlainUI    bool
		hflagProfile    string

		// Subcommand specific flags
		//
		// res:
		// flagResName
		// flagResType
		//
		// rg:
		// flagPattern
		//
		// query:
		// flagPattern
		// flagRecursive
		flagPattern   string
		flagRecursive bool
		flagResName   string
		flagResType   string
	)

	prepareConfigFile := func(ctx *cli.Context) error {
		// Prepare the config directory at $HOME/.aztfy
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("retrieving the user's HOME directory: %v", err)
		}
		configDir := filepath.Join(homeDir, cfgfile.CfgDirName)
		if err := os.MkdirAll(configDir, 0750); err != nil {
			return fmt.Errorf("creating the config directory at %s: %v", configDir, err)
		}
		configFile := filepath.Join(configDir, cfgfile.CfgFileName)

		_, err = os.Stat(configFile)
		if err == nil {
			return nil
		}
		if !os.IsNotExist(err) {
			return nil
		}

		// Generate a configuration file if not exist.

		// Get the installation id from following sources in order:
		// 1. The Azure CLI's configuration file
		// 2. The Azure PWSH's configuration file
		// 3. Generate one
		id, err := func() (string, error) {
			if id, err := cfgfile.GetInstallationIdFromCLI(); err == nil {
				return id, nil
			}
			log.Printf("[DEBUG] Installation ID not found from Azure CLI: %v", err)

			if id, err := cfgfile.GetInstallationIdFromPWSH(); err == nil {
				return id, nil
			}
			log.Printf("[DEBUG] Installation ID not found from Azure PWSH: %v", err)

			uuid, err := uuid.NewV4()
			if err != nil {
				return "", fmt.Errorf("generating installation id: %w", err)
			}
			return uuid.String(), nil
		}()

		if err != nil {
			return err
		}

		cfg := cfgfile.Configuration{
			InstallationId:   id,
			TelemetryEnabled: true,
		}
		b, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshalling the configuration file: %v", err)
		}
		// #nosec G306
		if err := os.WriteFile(configFile, b, 0644); err != nil {
			return fmt.Errorf("writing the configuration file: %v", err)
		}
		return nil
	}

	commandBeforeFunc := func(ctx *cli.Context) error {
		// Common flags check
		if flagAppend {
			if flagBackendType != "local" {
				return fmt.Errorf("`--append` only works for local backend")
			}
			if flagOverwrite {
				return fmt.Errorf("`--append` conflicts with `--overwrite`")
			}
		}
		if !flagNonInteractive {
			if flagContinue {
				return fmt.Errorf("`--continue` must be used together with `--non-interactive`")
			}
			if flagGenerateMappingFile {
				return fmt.Errorf("`--generate-mapping-file` must be used together with `--non-interactive`")
			}
		}
		if flagHCLOnly {
			if flagBackendType != "local" {
				return fmt.Errorf("`--hcl-only` only works for local backend")
			}
			if flagAppend {
				return fmt.Errorf("`--appned` conflicts with `--hcl-only`")
			}
			if flagModulePath != "" {
				return fmt.Errorf("`--module-path` conflicts with `--hcl-only`")
			}
		}
		if flagModulePath != "" {
			if !flagAppend {
				return fmt.Errorf("`--module-path` must be used together with `--append`")
			}
		}

		if flagLogLevel != "" {
			if _, err := logLevel(flagLogLevel); err != nil {
				return err
			}
		}

		// Initialize output directory
		if _, err := os.Stat(flagOutputDir); os.IsNotExist(err) {
			if err := os.MkdirAll(flagOutputDir, 0750); err != nil {
				return fmt.Errorf("creating output directory %q: %v", flagOutputDir, err)
			}
		}
		empty, err := utils.DirIsEmpty(flagOutputDir)
		if err != nil {
			return fmt.Errorf("failed to check emptiness of output directory %q: %v", flagOutputDir, err)
		}
		if !empty {
			switch {
			case flagOverwrite:
				if err := utils.RemoveEverythingUnder(flagOutputDir, meta.ResourceMappingFileName); err != nil {
					return fmt.Errorf("failed to clean up output directory %q: %v", flagOutputDir, err)
				}
			case flagAppend:
				// do nothing
			default:
				if flagNonInteractive {
					return fmt.Errorf("the output directory %q is not empty", flagOutputDir)
				}

				// Interactive mode
				fmt.Printf(`
The output directory is not empty. Please choose one of actions below:

* Press "Y" to overwrite the existing directory with new files
* Press "N" to append new files and add to the existing state instead
* Press other keys to quit

> `)
				var ans string
				// #nosec G104
				fmt.Scanf("%s", &ans)
				switch strings.ToLower(ans) {
				case "y":
					if err := utils.RemoveEverythingUnder(flagOutputDir, meta.ResourceMappingFileName); err != nil {
						return err
					}
				case "n":
					if flagHCLOnly {
						return fmt.Errorf("`--hcl-only` can only run within an empty directory")
					}
					flagAppend = true
				default:
					return fmt.Errorf("the output directory %q is not empty", flagOutputDir)
				}
			}
		}

		// Identify the subscription id, which comes from one of following (starts from the highest priority):
		// - Command line option
		// - Env variable: AZTFY_SUBSCRIPTION_ID
		// - Env variable: ARM_SUBSCRIPTION_ID
		// - Output of azure cli, the current active subscription
		if flagSubscriptionId == "" {
			var err error
			flagSubscriptionId, err = subscriptionIdFromCLI()
			if err != nil {
				return fmt.Errorf("retrieving subscription id from CLI: %v", err)
			}
		}

		return nil
	}

	commonFlags := []cli.Flag{
		&cli.StringFlag{
			Name: "subscription-id",
			// Honor the "ARM_SUBSCRIPTION_ID" as is used by the AzureRM provider, for easier use.
			EnvVars:     []string{"AZTFY_SUBSCRIPTION_ID", "ARM_SUBSCRIPTION_ID"},
			Aliases:     []string{"s"},
			Usage:       "The subscription id",
			Destination: &flagSubscriptionId,
		},
		&cli.StringFlag{
			Name:    "output-dir",
			EnvVars: []string{"AZTFY_OUTPUT_DIR"},
			Aliases: []string{"o"},
			Usage:   "The output directory (will create if not exists)",
			Value: func() string {
				dir, _ := os.Getwd()
				return dir
			}(),
			Destination: &flagOutputDir,
		},
		&cli.BoolFlag{
			Name:        "overwrite",
			EnvVars:     []string{"AZTFY_OVERWRITE"},
			Aliases:     []string{"f"},
			Usage:       "Whether to overwrite the output directory if it is not empty (use with caution)",
			Destination: &flagOverwrite,
		},
		&cli.BoolFlag{
			Name:        "append",
			EnvVars:     []string{"AZTFY_APPEND"},
			Usage:       "Skip cleaning up the output directory prior to importing, everything will be imported to the existing state file if any (local backend only)",
			Destination: &flagAppend,
		},
		&cli.BoolFlag{
			Name:        "dev-provider",
			EnvVars:     []string{"AZTFY_DEV_PROVIDER"},
			Usage:       fmt.Sprintf("Use the local development AzureRM provider, instead of the pinned provider in v%s", azurerm.ProviderSchemaInfo.Version),
			Destination: &flagDevProvider,
		},
		&cli.StringFlag{
			Name:        "backend-type",
			EnvVars:     []string{"AZTFY_BACKEND_TYPE"},
			Usage:       "The Terraform backend used to store the state",
			Value:       "local",
			Destination: &flagBackendType,
		},
		&cli.StringSliceFlag{
			Name:        "backend-config",
			EnvVars:     []string{"AZTFY_BACKEND_CONFIG"},
			Usage:       "The Terraform backend config",
			Destination: &flagBackendConfig,
		},
		&cli.BoolFlag{
			Name:        "full-properties",
			EnvVars:     []string{"AZTFY_FULL_PROPERTIES"},
			Usage:       "Whether to output all non-computed properties in the generated Terraform configuration? This probably needs manual modifications to make it valid",
			Value:       false,
			Destination: &flagFullConfig,
		},
		&cli.IntFlag{
			Name:        "parallelism",
			EnvVars:     []string{"AZTFY_PARALLELISM"},
			Usage:       "Limit the number of parallel operations, i.e., resource discovery, import",
			Value:       10,
			Destination: &flagParallelism,
		},
		&cli.BoolFlag{
			Name:        "non-interactive",
			EnvVars:     []string{"AZTFY_NON_INTERACTIVE"},
			Aliases:     []string{"n"},
			Usage:       "Non-interactive mode",
			Destination: &flagNonInteractive,
		},
		&cli.BoolFlag{
			Name:        "continue",
			EnvVars:     []string{"AZTFY_CONTINUE"},
			Aliases:     []string{"k"},
			Usage:       "In non-interactive mode, whether to continue on any import error",
			Destination: &flagContinue,
		},
		&cli.BoolFlag{
			Name:        "generate-mapping-file",
			Aliases:     []string{"g"},
			EnvVars:     []string{"AZTFY_GENERATE_MAPPING_FILE"},
			Usage:       "Only generate the resource mapping file, but DO NOT import any resource",
			Destination: &flagGenerateMappingFile,
		},
		&cli.BoolFlag{
			Name:        "hcl-only",
			EnvVars:     []string{"AZTFY_HCL_ONLY"},
			Usage:       "Only generate HCL code, but not the files for resource management (e.g. the state file)",
			Destination: &flagHCLOnly,
		},
		&cli.StringFlag{
			Name:        "module-path",
			EnvVars:     []string{"AZTFY_MODULE_PATH"},
			Usage:       `The path of the module (e.g. "module1.module2") where the resources will be imported and config generated. Note that only modules whose "source" is local path is supported. By default, it is the root module.`,
			Destination: &flagModulePath,
		},
		&cli.StringFlag{
			Name:        "log-path",
			EnvVars:     []string{"AZTFY_LOG_PATH"},
			Usage:       "The file path to store the log",
			Destination: &flagLogPath,
		},
		&cli.StringFlag{
			Name:        "log-level",
			EnvVars:     []string{"AZTFY_LOG_LEVEL"},
			Usage:       `Log level, can be one of "ERROR", "WARN", "INFO", "DEBUG" and "TRACE"`,
			Destination: &flagLogLevel,
			Value:       "INFO",
		},

		// Hidden flags
		&cli.BoolFlag{
			Name:        "mock-client",
			EnvVars:     []string{"AZTFY_MOCK_CLIENT"},
			Usage:       "Whether to mock the client. This is for testing UI",
			Hidden:      true,
			Destination: &hflagMockClient,
		},
		&cli.BoolFlag{
			Name:        "plain-ui",
			EnvVars:     []string{"AZTFY_PLAIN_UI"},
			Usage:       "In non-interactive mode, print the progress information line by line, rather than the spinner UI",
			Hidden:      true,
			Destination: &hflagPlainUI,
		},
		&cli.StringFlag{
			Name:        "profile",
			EnvVars:     []string{"AZTFY_PROFILE"},
			Usage:       "Profile the program, possible values are `cpu` and `memory`",
			Hidden:      true,
			Destination: &hflagProfile,
		},
	}

	resourceFlags := append([]cli.Flag{
		&cli.StringFlag{
			Name:        "name",
			EnvVars:     []string{"AZTFY_NAME"},
			Usage:       `The Terraform resource name.`,
			Value:       "res-0",
			Destination: &flagResName,
		},
		&cli.StringFlag{
			Name:        "type",
			EnvVars:     []string{"AZTFY_TYPE"},
			Usage:       `The Terraform resource type.`,
			Destination: &flagResType,
		},
	}, commonFlags...)

	resourceGroupFlags := append([]cli.Flag{
		&cli.StringFlag{
			Name:        "name-pattern",
			EnvVars:     []string{"AZTFY_NAME_PATTERN"},
			Aliases:     []string{"p"},
			Usage:       `The pattern of the resource name. The semantic of a pattern is the same as Go's os.CreateTemp()`,
			Value:       "res-",
			Destination: &flagPattern,
		},
	}, commonFlags...)

	queryFlags := append([]cli.Flag{
		&cli.BoolFlag{
			Name:        "recursive",
			EnvVars:     []string{"AZTFY_RECURSIVE"},
			Aliases:     []string{"r"},
			Usage:       "Whether to recursively list child resources of the query result",
			Destination: &flagRecursive,
		},
	}, resourceGroupFlags...)

	mappingFileFlags := append([]cli.Flag{}, commonFlags...)

	safeOutputFileNames := config.OutputFileNames{
		TerraformFileName: "terraform.aztfy.tf",
		ProviderFileName:  "provider.aztfy.tf",
		MainFileName:      "main.aztfy.tf",
	}

	app := &cli.App{
		Name:      "aztfy",
		Version:   getVersion(),
		Usage:     "Bring existing Azure resources under Terraform's management",
		UsageText: "aztfy [command] [option]",
		Before:    prepareConfigFile,
		Commands: []*cli.Command{
			{
				Name:      "config",
				Usage:     `aztfy configuration command`,
				UsageText: "aztfy config [subcommand]",
				Subcommands: []*cli.Command{
					{
						Name:      "set",
						Usage:     `Set a configuration item for aztfy`,
						UsageText: "aztfy config set key value",
						Action: func(c *cli.Context) error {
							if c.NArg() != 2 {
								return fmt.Errorf("Please specify a configuration key and value")
							}

							key := c.Args().Get(0)
							value := c.Args().Get(1)

							return cfgfile.SetKey(key, value)
						},
					},
					{
						Name:      "get",
						Usage:     `Get a configuration item for aztfy`,
						UsageText: "aztfy config get key",
						Action: func(c *cli.Context) error {
							if c.NArg() != 1 {
								return fmt.Errorf("Please specify a configuration key")
							}

							key := c.Args().Get(0)
							v, err := cfgfile.GetKey(key)
							if err != nil {
								return err
							}
							fmt.Println(v)
							return nil
						},
					},
					{
						Name:      "show",
						Usage:     `Show the full configuration for aztfy`,
						UsageText: "aztfy config show",
						Action: func(c *cli.Context) error {
							cfg, err := cfgfile.GetConfig()
							if err != nil {
								return err
							}
							b, err := json.MarshalIndent(cfg, "", "  ")
							if err != nil {
								return err
							}
							fmt.Println(string(b))
							return nil
						},
					},
				},
			},
			{
				Name:      "resource",
				Aliases:   []string{"res"},
				Usage:     "Terrafying a single resource",
				UsageText: "aztfy resource [option] <resource id>",
				Flags:     resourceFlags,
				Before:    commandBeforeFunc,
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No resource id specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one resource ids specified")
					}

					resId := c.Args().First()

					if _, err := armid.ParseResourceId(resId); err != nil {
						return fmt.Errorf("invalid resource id: %v", err)
					}

					cred, clientOpt, err := buildAzureSDKCredAndClientOpt()
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig: config.CommonConfig{
							SubscriptionId:       flagSubscriptionId,
							AzureSDKCredential:   cred,
							AzureSDKClientOption: *clientOpt,
							OutputDir:            flagOutputDir,
							DevProvider:          flagDevProvider,
							ContinueOnError:      flagContinue,
							BackendType:          flagBackendType,
							BackendConfig:        flagBackendConfig.Value(),
							FullConfig:           flagFullConfig,
							Parallelism:          flagParallelism,
							HCLOnly:              flagHCLOnly,
							ModulePath:           flagModulePath,
							TelemetryClient:      initTelemetryClient(),
						},
						ResourceId:     resId,
						TFResourceName: flagResName,
						TFResourceType: flagResType,
					}

					if flagAppend {
						cfg.CommonConfig.OutputFileNames = safeOutputFileNames
					}

					return realMain(c.Context, cfg, flagNonInteractive, hflagMockClient, hflagPlainUI, flagGenerateMappingFile, hflagProfile)
				},
			},
			{
				Name:      "resource-group",
				Aliases:   []string{"rg"},
				Usage:     "Terrafying a resource group and the nested resources resides within it",
				UsageText: "aztfy resource-group [option] <resource group name>",
				Flags:     resourceGroupFlags,
				Before:    commandBeforeFunc,
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No resource group specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one resource groups specified")
					}

					rg := c.Args().First()

					cred, clientOpt, err := buildAzureSDKCredAndClientOpt()
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig: config.CommonConfig{
							SubscriptionId:       flagSubscriptionId,
							AzureSDKCredential:   cred,
							AzureSDKClientOption: *clientOpt,
							OutputDir:            flagOutputDir,
							DevProvider:          flagDevProvider,
							ContinueOnError:      flagContinue,
							BackendType:          flagBackendType,
							BackendConfig:        flagBackendConfig.Value(),
							FullConfig:           flagFullConfig,
							Parallelism:          flagParallelism,
							HCLOnly:              flagHCLOnly,
							ModulePath:           flagModulePath,
							TelemetryClient:      initTelemetryClient(),
						},
						ResourceGroupName:   rg,
						ResourceNamePattern: flagPattern,
						RecursiveQuery:      true,
					}

					if flagAppend {
						cfg.CommonConfig.OutputFileNames = safeOutputFileNames
					}

					return realMain(c.Context, cfg, flagNonInteractive, hflagMockClient, hflagPlainUI, flagGenerateMappingFile, hflagProfile)
				},
			},
			{
				Name:      "query",
				Usage:     "Terrafying a customized scope of resources determined by an Azure Resource Graph where predicate",
				UsageText: "aztfy query [option] <ARG where predicate>",
				Flags:     queryFlags,
				Before:    commandBeforeFunc,
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No query specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one queries specified")
					}

					predicate := c.Args().First()

					cred, clientOpt, err := buildAzureSDKCredAndClientOpt()
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig: config.CommonConfig{
							SubscriptionId:       flagSubscriptionId,
							AzureSDKCredential:   cred,
							AzureSDKClientOption: *clientOpt,
							OutputDir:            flagOutputDir,
							DevProvider:          flagDevProvider,
							ContinueOnError:      flagContinue,
							BackendType:          flagBackendType,
							BackendConfig:        flagBackendConfig.Value(),
							FullConfig:           flagFullConfig,
							Parallelism:          flagParallelism,
							HCLOnly:              flagHCLOnly,
							ModulePath:           flagModulePath,
							TelemetryClient:      initTelemetryClient(),
						},
						ARGPredicate:        predicate,
						ResourceNamePattern: flagPattern,
						RecursiveQuery:      flagRecursive,
					}

					if flagAppend {
						cfg.CommonConfig.OutputFileNames = safeOutputFileNames
					}

					return realMain(c.Context, cfg, flagNonInteractive, hflagMockClient, hflagPlainUI, flagGenerateMappingFile, hflagProfile)
				},
			},
			{
				Name:      "mapping-file",
				Aliases:   []string{"map"},
				Usage:     "Terrafying a customized scope of resources determined by the resource mapping file",
				UsageText: "aztfy mapping-file [option] <resource mapping file>",
				Flags:     mappingFileFlags,
				Before:    commandBeforeFunc,
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No resource mapping file specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one resource mapping files specified")
					}

					mapFile := c.Args().First()

					cred, clientOpt, err := buildAzureSDKCredAndClientOpt()
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig: config.CommonConfig{
							SubscriptionId:       flagSubscriptionId,
							AzureSDKCredential:   cred,
							AzureSDKClientOption: *clientOpt,
							OutputDir:            flagOutputDir,
							DevProvider:          flagDevProvider,
							ContinueOnError:      flagContinue,
							BackendType:          flagBackendType,
							BackendConfig:        flagBackendConfig.Value(),
							FullConfig:           flagFullConfig,
							Parallelism:          flagParallelism,
							HCLOnly:              flagHCLOnly,
							ModulePath:           flagModulePath,
							TelemetryClient:      initTelemetryClient(),
						},
						MappingFile: mapFile,
					}

					if flagAppend {
						cfg.CommonConfig.OutputFileNames = safeOutputFileNames
					}

					return realMain(c.Context, cfg, flagNonInteractive, hflagMockClient, hflagPlainUI, flagGenerateMappingFile, hflagProfile)
				},
			},
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func logLevel(level string) (hclog.Level, error) {
	switch level {
	case "ERROR":
		return hclog.Error, nil
	case "WARN":
		return hclog.Warn, nil
	case "INFO":
		return hclog.Info, nil
	case "DEBUG":
		return hclog.Debug, nil
	case "TRACE":
		return hclog.Trace, nil
	default:
		return hclog.NoLevel, fmt.Errorf("unknown log level: %s", level)
	}
}

func initLog(path string, flagLevel string) error {
	golog.SetOutput(io.Discard)

	level, err := logLevel(flagLevel)
	if err != nil {
		return err
	}

	if path != "" {
		// #nosec G304
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return fmt.Errorf("creating log file %s: %v", path, err)
		}

		logger := hclog.New(&hclog.LoggerOptions{
			Name:   "aztfy",
			Level:  level,
			Output: f,
		}).StandardLogger(&hclog.StandardLoggerOptions{
			InferLevels: true,
		})

		// Enable log for aztfy
		log.SetLogger(logger)

		// Enable log for azlist
		azlist.SetLogger(logger)

		// Enable log for azure sdk
		os.Setenv("AZURE_SDK_GO_LOGGING", "all") // #nosec G104
		azlog.SetListener(func(cls azlog.Event, msg string) {
			logger.Printf("[TRACE] %s: %s\n", cls, msg)
		})
	}
	return nil
}

func initTelemetryClient() telemetry.Client {
	cfg, err := cfgfile.GetConfig()
	if err != nil {
		return telemetry.NewNullClient()
	}
	enabled, id := cfg.TelemetryEnabled, cfg.InstallationId
	if !enabled {
		return telemetry.NewNullClient()
	}
	if id == "" {
		uuid, err := uuid.NewV4()
		if err == nil {
			id = uuid.String()
		} else {
			id = "undefined"
		}
	}

	sessionId := "undefined"
	if uuid, err := uuid.NewV4(); err == nil {
		sessionId = uuid.String()
	}
	return telemetry.NewAppInsight(id, sessionId)
}

// buildAzureSDKCredAndClientOpt builds the Azure SDK credential and client option from multiple sources (i.e. environment variables, MSI, Azure CLI).
func buildAzureSDKCredAndClientOpt() (azcore.TokenCredential, *arm.ClientOptions, error) {
	env := "public"
	if v := os.Getenv("ARM_ENVIRONMENT"); v != "" {
		env = v
	}

	var cloudCfg cloud.Configuration
	switch strings.ToLower(env) {
	case "public":
		cloudCfg = cloud.AzurePublic
	case "usgovernment":
		cloudCfg = cloud.AzureGovernment
	case "china":
		cloudCfg = cloud.AzureChina
	default:
		return nil, nil, fmt.Errorf("unknown environment specified: %q", env)
	}

	// Maps the auth related environment variables used in the provider to what azidentity honors
	if v, ok := os.LookupEnv("ARM_TENANT_ID"); ok {
		// #nosec G104
		os.Setenv("AZURE_TENANT_ID", v)
	}
	if v, ok := os.LookupEnv("ARM_CLIENT_ID"); ok {
		// #nosec G104
		os.Setenv("AZURE_CLIENT_ID", v)
	}
	if v, ok := os.LookupEnv("ARM_CLIENT_SECRET"); ok {
		// #nosec G104
		os.Setenv("AZURE_CLIENT_SECRET", v)
	}
	if v, ok := os.LookupEnv("ARM_CLIENT_CERTIFICATE_PATH"); ok {
		// #nosec G104
		os.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", v)
	}

	clientOpt := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloudCfg,
			Telemetry: policy.TelemetryOptions{
				ApplicationID: "aztfy",
				Disabled:      false,
			},
			Logging: policy.LogOptions{
				IncludeBody: true,
			},
		},
	}

	cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
		ClientOptions: clientOpt.ClientOptions,
		TenantID:      os.Getenv("ARM_TENANT_ID"),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to obtain a credential: %v", err)
	}

	return cred, clientOpt, nil
}

func subscriptionIdFromCLI() (string, error) {
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd := exec.Command("az", "account", "show", "--output", "json", "--query", "id")
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		err = fmt.Errorf("failed to run azure cli: %v", err)
		if stdErrStr := stderr.String(); stdErrStr != "" {
			err = fmt.Errorf("%s: %s", err, strings.TrimSpace(stdErrStr))
		}
		return "", err
	}
	if stdout.String() == "" {
		return "", fmt.Errorf("subscription id is not specified")
	}
	return strconv.Unquote(strings.TrimSpace(stdout.String()))
}

func realMain(ctx context.Context, cfg config.Config, batch, mockMeta, plainUI, genMapFile bool, profileType string) (result error) {
	switch strings.ToLower(profileType) {
	case "cpu":
		defer profile.Start(profile.CPUProfile, profile.ProfilePath("."), profile.NoShutdownHook).Stop()
	case "mem":
		defer profile.Start(profile.MemProfile, profile.ProfilePath("."), profile.NoShutdownHook).Stop()
	}

	// Initialize log
	if err := initLog(flagLogPath, flagLogLevel); err != nil {
		result = err
		return
	}

	tc := cfg.TelemetryClient

	defer func() {
		if result == nil {
			log.Printf("[INFO] aztfy ends")
			tc.Trace(telemetry.Info, "aztfy ends")
		} else {
			log.Printf("[ERROR] aztfy ends with error: %v", result)
			tc.Trace(telemetry.Error, fmt.Sprintf("aztfy ends with error: %v", result))
		}
		tc.Close()
	}()

	log.Printf("[INFO] aztfy starts with config: %#v", cfg)
	tc.Trace(telemetry.Info, "aztfy starts")

	// Run in non-interactive mode
	if batch {
		nicfg := internalconfig.NonInteractiveModeConfig{
			MockMeta:           mockMeta,
			Config:             cfg,
			PlainUI:            plainUI,
			GenMappingFileOnly: genMapFile,
		}
		if err := internal.BatchImport(ctx, nicfg); err != nil {
			result = err
			return
		}
		return nil
	}

	// Run in interactive mode
	icfg := internalconfig.InteractiveModeConfig{
		Config:   cfg,
		MockMeta: mockMeta,
	}
	prog, err := ui.NewProgram(ctx, icfg)
	if err != nil {
		result = err
		return
	}
	if err := prog.Start(); err != nil {
		result = err
		return
	}
	return nil
}
