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

	"github.com/Azure/aztfexport/internal/cfgfile"
	internalconfig "github.com/Azure/aztfexport/internal/config"
	"github.com/Azure/aztfexport/pkg/telemetry"
	"github.com/gofrs/uuid"
	"github.com/pkg/profile"

	"github.com/Azure/aztfexport/pkg/config"
	"github.com/Azure/aztfexport/pkg/log"

	"github.com/hashicorp/go-hclog"
	"github.com/magodo/armid"
	"github.com/magodo/azlist/azlist"
	"github.com/magodo/terraform-client-go/tfclient"
	"github.com/magodo/tfadd/providers/azurerm"

	"github.com/Azure/aztfexport/internal"
	"github.com/Azure/aztfexport/internal/ui"
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

func prepareConfigFile(ctx *cli.Context) error {
	// Prepare the config directory at $HOME/.aztfexport
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

func main() {
	commonFlags := []cli.Flag{
		&cli.StringFlag{
			Name: "env",
			// Honor the "ARM_ENVIRONMENT" as is used by the AzureRM provider, for easier use.
			EnvVars:     []string{"AZTFEXPORT_ENV", "ARM_ENVIRONMENT"},
			Usage:       `The cloud environment, can be one of "public", "usgovernment" and "china"`,
			Destination: &flagset.flagEnv,
			Value:       "public",
		},
		&cli.StringFlag{
			Name: "subscription-id",
			// Honor the "ARM_SUBSCRIPTION_ID" as is used by the AzureRM provider, for easier use.
			EnvVars:     []string{"AZTFEXPORT_SUBSCRIPTION_ID", "ARM_SUBSCRIPTION_ID"},
			Aliases:     []string{"s"},
			Usage:       "The subscription id",
			Destination: &flagset.flagSubscriptionId,
		},
		&cli.BoolFlag{
			Name:        "use-environment-cred",
			EnvVars:     []string{"AZTFEXPORT_USE_ENVIRONMENT_CRED"},
			Usage:       "Explicitly use the environment variables to do authentication",
			Destination: &flagset.flagUseEnvironmentCred,
		},
		&cli.BoolFlag{
			Name:        "use-managed-identity-cred",
			EnvVars:     []string{"AZTFEXPORT_USE_MANAGED_IDENTITY_CRED"},
			Usage:       "Explicitly use the managed identity that is provided by the Azure host to do authentication",
			Destination: &flagset.flagUseManagedIdentityCred,
		},
		&cli.BoolFlag{
			Name:        "use-azure-cli-cred",
			EnvVars:     []string{"AZTFEXPORT_USE_AZURE_CLI_CRED"},
			Usage:       "Explicitly use the Azure CLI to do authentication",
			Destination: &flagset.flagUseAzureCLICred,
		},
		&cli.StringFlag{
			Name:    "output-dir",
			EnvVars: []string{"AZTFEXPORT_OUTPUT_DIR"},
			Aliases: []string{"o"},
			Usage:   "The output directory (will create the dir if it does not exist)",
			Value: func() string {
				dir, _ := os.Getwd()
				return dir
			}(),
			Destination: &flagset.flagOutputDir,
		},
		&cli.BoolFlag{
			Name:        "overwrite",
			EnvVars:     []string{"AZTFEXPORT_OVERWRITE"},
			Aliases:     []string{"f"},
			Usage:       "Overwrites the output directory if it is not empty (use with caution)",
			Destination: &flagset.flagOverwrite,
		},
		&cli.BoolFlag{
			Name:        "append",
			EnvVars:     []string{"AZTFEXPORT_APPEND"},
			Usage:       "Imports to the existing state file if any and does not clean up the output directory",
			Destination: &flagset.flagAppend,
		},
		&cli.BoolFlag{
			Name:        "dev-provider",
			EnvVars:     []string{"AZTFEXPORT_DEV_PROVIDER"},
			Usage:       fmt.Sprintf("Use the local development AzureRM provider, instead of the pinned provider in v%s", azurerm.ProviderSchemaInfo.Version),
			Destination: &flagset.flagDevProvider,
		},
		&cli.StringFlag{
			Name:        "provider-version",
			EnvVars:     []string{"AZTFEXPORT_PROVIDER_VERSION"},
			Usage:       fmt.Sprintf("The azurerm provider version to use for importing (default: existing version constraints or %s)", azurerm.ProviderSchemaInfo.Version),
			Destination: &flagset.flagProviderVersion,
		},
		&cli.StringFlag{
			Name:        "backend-type",
			EnvVars:     []string{"AZTFEXPORT_BACKEND_TYPE"},
			Usage:       "The Terraform backend used to store the state (default: local)",
			Destination: &flagset.flagBackendType,
		},
		&cli.StringSliceFlag{
			Name:        "backend-config",
			EnvVars:     []string{"AZTFEXPORT_BACKEND_CONFIG"},
			Usage:       "The Terraform backend config",
			Destination: &flagset.flagBackendConfig,
		},
		&cli.BoolFlag{
			Name:        "full-properties",
			EnvVars:     []string{"AZTFEXPORT_FULL_PROPERTIES"},
			Usage:       "Includes all non-computed properties in the Terraform configuration. This may require manual modifications to produce a valid config",
			Value:       false,
			Destination: &flagset.flagFullConfig,
		},
		&cli.IntFlag{
			Name:        "parallelism",
			EnvVars:     []string{"AZTFEXPORT_PARALLELISM"},
			Usage:       "Limit the number of parallel operations, i.e., resource discovery, import",
			Value:       10,
			Destination: &flagset.flagParallelism,
		},
		&cli.BoolFlag{
			Name:        "non-interactive",
			EnvVars:     []string{"AZTFEXPORT_NON_INTERACTIVE"},
			Aliases:     []string{"n"},
			Usage:       "Non-interactive mode",
			Destination: &flagset.flagNonInteractive,
		},
		&cli.BoolFlag{
			Name:        "plain-ui",
			EnvVars:     []string{"AZTFEXPORT_PLAIN_UI"},
			Usage:       "In non-interactive mode, print the progress information line by line, rather than the spinner UI. This can be used in OS that has no /dev/tty available",
			Destination: &flagset.flagPlainUI,
		},
		&cli.BoolFlag{
			Name:        "continue",
			EnvVars:     []string{"AZTFEXPORT_CONTINUE"},
			Aliases:     []string{"k"},
			Usage:       "For non-interactive mode, continue on any import error",
			Destination: &flagset.flagContinue,
		},
		&cli.BoolFlag{
			Name:        "generate-mapping-file",
			Aliases:     []string{"g"},
			EnvVars:     []string{"AZTFEXPORT_GENERATE_MAPPING_FILE"},
			Usage:       "Only generate the resource mapping file, but does NOT import any resource",
			Destination: &flagset.flagGenerateMappingFile,
		},
		&cli.BoolFlag{
			Name:        "hcl-only",
			EnvVars:     []string{"AZTFEXPORT_HCL_ONLY"},
			Usage:       "Only generates HCL code (and mapping file), but not the files for resource management (e.g. the state file)",
			Destination: &flagset.flagHCLOnly,
		},
		&cli.StringFlag{
			Name:        "module-path",
			EnvVars:     []string{"AZTFEXPORT_MODULE_PATH"},
			Usage:       `The path of the module (e.g. "module1.module2") where the resources will be imported and config generated. Note that only modules whose "source" is local path is supported. Defaults to the root module.`,
			Destination: &flagset.flagModulePath,
		},
		&cli.StringFlag{
			Name:        "log-path",
			EnvVars:     []string{"AZTFEXPORT_LOG_PATH"},
			Usage:       "The file path to store the log",
			Destination: &flagLogPath,
		},
		&cli.StringFlag{
			Name:        "log-level",
			EnvVars:     []string{"AZTFEXPORT_LOG_LEVEL"},
			Usage:       `Log level, can be one of "ERROR", "WARN", "INFO", "DEBUG" and "TRACE"`,
			Destination: &flagLogLevel,
			Value:       "INFO",
		},

		// Hidden flags
		&cli.BoolFlag{
			Name:        "mock-client",
			EnvVars:     []string{"AZTFEXPORT_MOCK_CLIENT"},
			Usage:       "Whether to mock the client. This is for testing UI",
			Hidden:      true,
			Destination: &flagset.hflagMockClient,
		},
		&cli.StringFlag{
			Name:        "profile",
			EnvVars:     []string{"AZTFEXPORT_PROFILE"},
			Usage:       "Profile the program, possible values are `cpu` and `memory`",
			Hidden:      true,
			Destination: &flagset.hflagProfile,
		},
		&cli.StringFlag{
			Name:        "tfclient-plugin-path",
			EnvVars:     []string{"AZTFEXPORT_TFCLIENT_PLUGIN_PATH"},
			Usage:       "Replace terraform binary with terraform-client-go for importing (must be used with `--hcl-only`)",
			Hidden:      true,
			Destination: &flagset.hflagTFClientPluginPath,
		},
	}

	resourceFlags := append([]cli.Flag{
		&cli.StringFlag{
			Name:        "name",
			EnvVars:     []string{"AZTFEXPORT_NAME"},
			Usage:       `The Terraform resource name.`,
			Value:       "res-0",
			Destination: &flagset.flagResName,
		},
		&cli.StringFlag{
			Name:        "type",
			EnvVars:     []string{"AZTFEXPORT_TYPE"},
			Usage:       `The Terraform resource type.`,
			Destination: &flagset.flagResType,
		},
	}, commonFlags...)

	resourceGroupFlags := append([]cli.Flag{
		&cli.StringFlag{
			Name:        "name-pattern",
			EnvVars:     []string{"AZTFEXPORT_NAME_PATTERN"},
			Aliases:     []string{"p"},
			Usage:       `The pattern of the resource name. The semantic of a pattern is the same as Go's os.CreateTemp()`,
			Value:       "res-",
			Destination: &flagset.flagPattern,
		},
	}, commonFlags...)

	queryFlags := append([]cli.Flag{
		&cli.BoolFlag{
			Name:        "recursive",
			EnvVars:     []string{"AZTFEXPORT_RECURSIVE"},
			Aliases:     []string{"r"},
			Usage:       "Recursively lists child resources of the resulting query resources",
			Destination: &flagset.flagRecursive,
		},
	}, resourceGroupFlags...)

	mappingFileFlags := append([]cli.Flag{}, commonFlags...)

	safeOutputFileNames := config.OutputFileNames{
		TerraformFileName:   "terraform.aztfexport.tf",
		ProviderFileName:    "provider.aztfexport.tf",
		MainFileName:        "main.aztfexport.tf",
		ImportBlockFileName: "import.aztfexport.tf",
	}

	app := &cli.App{
		Name:      "aztfexport",
		Version:   getVersion(),
		Usage:     "A tool to bring existing Azure resources under Terraform's management",
		UsageText: "aztfexport <command> [option] <scope>",
		Before:    prepareConfigFile,
		Commands: []*cli.Command{
			{
				Name:      "config",
				Usage:     `Configuring the tool`,
				UsageText: "aztfexport config [subcommand]",
				Subcommands: []*cli.Command{
					{
						Name:      "set",
						Usage:     `Set a configuration item for aztfexport`,
						UsageText: "aztfexport config set key value",
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
						Usage:     `Get a configuration item for aztfexport`,
						UsageText: "aztfexport config get key",
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
						Usage:     `Show the full configuration for aztfexport`,
						UsageText: "aztfexport config show",
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
				Name:      ModeResource,
				Aliases:   []string{"res"},
				Usage:     "Exporting a single resource",
				UsageText: "aztfexport resource [option] <resource id>",
				Flags:     resourceFlags,
				Before:    commandBeforeFunc(&flagset),
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

					cred, clientOpt, err := buildAzureSDKCredAndClientOpt(flagset.flagEnv, NewAuthMethodFromFlagSet(flagset))
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig: config.CommonConfig{
							SubscriptionId:       flagset.flagSubscriptionId,
							AzureSDKCredential:   cred,
							AzureSDKClientOption: *clientOpt,
							OutputDir:            flagset.flagOutputDir,
							ProviderVersion:      flagset.flagProviderVersion,
							DevProvider:          flagset.flagDevProvider,
							ContinueOnError:      flagset.flagContinue,
							BackendType:          flagset.flagBackendType,
							BackendConfig:        flagset.flagBackendConfig.Value(),
							FullConfig:           flagset.flagFullConfig,
							Parallelism:          flagset.flagParallelism,
							HCLOnly:              flagset.flagHCLOnly,
							ModulePath:           flagset.flagModulePath,
							TelemetryClient:      initTelemetryClient(flagset.flagSubscriptionId),
						},
						ResourceId:     resId,
						TFResourceName: flagset.flagResName,
						TFResourceType: flagset.flagResType,
					}

					if flagset.flagAppend {
						cfg.CommonConfig.OutputFileNames = safeOutputFileNames
					}

					if flagset.hflagTFClientPluginPath != "" {
						// #nosec G204
						tfc, err := tfclient.New(tfclient.Option{
							Cmd:    exec.Command(flagset.hflagTFClientPluginPath),
							Logger: hclog.NewNullLogger(),
						})
						if err != nil {
							return err
						}
						cfg.CommonConfig.TFClient = tfc
					}

					return realMain(c.Context, cfg, flagset.flagNonInteractive, flagset.hflagMockClient, flagset.flagPlainUI, flagset.flagGenerateMappingFile, flagset.hflagProfile, flagset.DescribeCLI(ModeResource))
				},
			},
			{
				Name:      ModeResourceGroup,
				Aliases:   []string{"rg"},
				Usage:     "Exporting a resource group and the nested resources resides within it",
				UsageText: "aztfexport resource-group [option] <resource group name>",
				Flags:     resourceGroupFlags,
				Before:    commandBeforeFunc(&flagset),
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No resource group specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one resource groups specified")
					}

					rg := c.Args().First()

					cred, clientOpt, err := buildAzureSDKCredAndClientOpt(flagset.flagEnv, NewAuthMethodFromFlagSet(flagset))
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig: config.CommonConfig{
							SubscriptionId:       flagset.flagSubscriptionId,
							AzureSDKCredential:   cred,
							AzureSDKClientOption: *clientOpt,
							OutputDir:            flagset.flagOutputDir,
							ProviderVersion:      flagset.flagProviderVersion,
							DevProvider:          flagset.flagDevProvider,
							ContinueOnError:      flagset.flagContinue,
							BackendType:          flagset.flagBackendType,
							BackendConfig:        flagset.flagBackendConfig.Value(),
							FullConfig:           flagset.flagFullConfig,
							Parallelism:          flagset.flagParallelism,
							HCLOnly:              flagset.flagHCLOnly,
							ModulePath:           flagset.flagModulePath,
							TelemetryClient:      initTelemetryClient(flagset.flagSubscriptionId),
						},
						ResourceGroupName:   rg,
						ResourceNamePattern: flagset.flagPattern,
						RecursiveQuery:      true,
					}

					if flagset.flagAppend {
						cfg.CommonConfig.OutputFileNames = safeOutputFileNames
					}

					if flagset.hflagTFClientPluginPath != "" {
						// #nosec G204
						tfc, err := tfclient.New(tfclient.Option{
							Cmd:    exec.Command(flagset.hflagTFClientPluginPath),
							Logger: hclog.NewNullLogger(),
						})
						if err != nil {
							return err
						}
						cfg.CommonConfig.TFClient = tfc
					}

					return realMain(c.Context, cfg, flagset.flagNonInteractive, flagset.hflagMockClient, flagset.flagPlainUI, flagset.flagGenerateMappingFile, flagset.hflagProfile, flagset.DescribeCLI(ModeResourceGroup))
				},
			},
			{
				Name:      ModeQuery,
				Usage:     "Exporting a customized scope of resources determined by an Azure Resource Graph where predicate",
				UsageText: "aztfexport query [option] <ARG where predicate>",
				Flags:     queryFlags,
				Before:    commandBeforeFunc(&flagset),
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No query specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one queries specified. Use `and` with double quotes to run multiple query parameters.")
					}

					predicate := c.Args().First()

					cred, clientOpt, err := buildAzureSDKCredAndClientOpt(flagset.flagEnv, NewAuthMethodFromFlagSet(flagset))
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig: config.CommonConfig{
							SubscriptionId:       flagset.flagSubscriptionId,
							AzureSDKCredential:   cred,
							AzureSDKClientOption: *clientOpt,
							OutputDir:            flagset.flagOutputDir,
							ProviderVersion:      flagset.flagProviderVersion,
							DevProvider:          flagset.flagDevProvider,
							ContinueOnError:      flagset.flagContinue,
							BackendType:          flagset.flagBackendType,
							BackendConfig:        flagset.flagBackendConfig.Value(),
							FullConfig:           flagset.flagFullConfig,
							Parallelism:          flagset.flagParallelism,
							HCLOnly:              flagset.flagHCLOnly,
							ModulePath:           flagset.flagModulePath,
							TelemetryClient:      initTelemetryClient(flagset.flagSubscriptionId),
						},
						ARGPredicate:        predicate,
						ResourceNamePattern: flagset.flagPattern,
						RecursiveQuery:      flagset.flagRecursive,
					}

					if flagset.flagAppend {
						cfg.CommonConfig.OutputFileNames = safeOutputFileNames
					}

					if flagset.hflagTFClientPluginPath != "" {
						// #nosec G204
						tfc, err := tfclient.New(tfclient.Option{
							Cmd:    exec.Command(flagset.hflagTFClientPluginPath),
							Logger: hclog.NewNullLogger(),
						})
						if err != nil {
							return err
						}
						cfg.CommonConfig.TFClient = tfc
					}

					return realMain(c.Context, cfg, flagset.flagNonInteractive, flagset.hflagMockClient, flagset.flagPlainUI, flagset.flagGenerateMappingFile, flagset.hflagProfile, flagset.DescribeCLI(ModeQuery))
				},
			},
			{
				Name:      ModeMappingFile,
				Aliases:   []string{"map"},
				Usage:     "Exporting a customized scope of resources determined by the resource mapping file",
				UsageText: "aztfexport mapping-file [option] <resource mapping file>",
				Flags:     mappingFileFlags,
				Before:    commandBeforeFunc(&flagset),
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No resource mapping file specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one resource mapping files specified")
					}

					mapFile := c.Args().First()

					cred, clientOpt, err := buildAzureSDKCredAndClientOpt(flagset.flagEnv, NewAuthMethodFromFlagSet(flagset))
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig: config.CommonConfig{
							SubscriptionId:       flagset.flagSubscriptionId,
							AzureSDKCredential:   cred,
							AzureSDKClientOption: *clientOpt,
							OutputDir:            flagset.flagOutputDir,
							ProviderVersion:      flagset.flagProviderVersion,
							DevProvider:          flagset.flagDevProvider,
							ContinueOnError:      flagset.flagContinue,
							BackendType:          flagset.flagBackendType,
							BackendConfig:        flagset.flagBackendConfig.Value(),
							FullConfig:           flagset.flagFullConfig,
							Parallelism:          flagset.flagParallelism,
							HCLOnly:              flagset.flagHCLOnly,
							ModulePath:           flagset.flagModulePath,
							TelemetryClient:      initTelemetryClient(flagset.flagSubscriptionId),
						},
						MappingFile: mapFile,
					}

					if flagset.flagAppend {
						cfg.CommonConfig.OutputFileNames = safeOutputFileNames
					}

					if flagset.hflagTFClientPluginPath != "" {
						// #nosec G204
						tfc, err := tfclient.New(tfclient.Option{
							Cmd:    exec.Command(flagset.hflagTFClientPluginPath),
							Logger: hclog.NewNullLogger(),
						})
						if err != nil {
							return err
						}
						cfg.CommonConfig.TFClient = tfc
					}

					return realMain(c.Context, cfg, flagset.flagNonInteractive, flagset.hflagMockClient, flagset.flagPlainUI, flagset.flagGenerateMappingFile, flagset.hflagProfile, flagset.DescribeCLI(ModeMappingFile))
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
			Name:   "aztfexport",
			Level:  level,
			Output: f,
		}).StandardLogger(&hclog.StandardLoggerOptions{
			InferLevels: true,
		})

		// Enable log for aztfexport
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

func initTelemetryClient(subscriptionId string) telemetry.Client {
	cfg, err := cfgfile.GetConfig()
	if err != nil {
		return telemetry.NewNullClient()
	}
	enabled, installId := cfg.TelemetryEnabled, cfg.InstallationId
	if !enabled {
		return telemetry.NewNullClient()
	}
	if installId == "" {
		uuid, err := uuid.NewV4()
		if err == nil {
			installId = uuid.String()
		} else {
			installId = "undefined"
		}
	}

	sessionId := "undefined"
	if uuid, err := uuid.NewV4(); err == nil {
		sessionId = uuid.String()
	}
	return telemetry.NewAppInsight(subscriptionId, installId, sessionId)
}

// At most one of below is true
type authMethod int

const (
	authMethodDefault authMethod = iota
	authMethodEnvironment
	authMethodManagedIdentity
	authMethodAzureCLI
)

func NewAuthMethodFromFlagSet(fset FlagSet) authMethod {
	if fset.flagUseEnvironmentCred {
		return authMethodEnvironment
	}
	if fset.flagUseManagedIdentityCred {
		return authMethodManagedIdentity
	}
	if fset.flagUseAzureCLICred {
		return authMethodAzureCLI
	}
	return authMethodDefault
}

// buildAzureSDKCredAndClientOpt builds the Azure SDK credential and client option from multiple sources (i.e. environment variables, MSI, Azure CLI).
func buildAzureSDKCredAndClientOpt(env string, authMethod authMethod) (azcore.TokenCredential, *arm.ClientOptions, error) {
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
				ApplicationID: "aztfexport",
				Disabled:      false,
			},
			Logging: policy.LogOptions{
				IncludeBody: true,
			},
		},
	}

	tenantId := os.Getenv("ARM_TENANT_ID")
	var (
		cred azcore.TokenCredential
		err  error
	)
	switch authMethod {
	case authMethodEnvironment:
		cred, err = azidentity.NewEnvironmentCredential(&azidentity.EnvironmentCredentialOptions{
			ClientOptions: clientOpt.ClientOptions,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to new Environment credential: %v", err)
		}
		return cred, clientOpt, nil
	case authMethodManagedIdentity:
		cred, err = azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
			ClientOptions: clientOpt.ClientOptions,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to new Managed Identity credential: %v", err)
		}
		return cred, clientOpt, nil
	case authMethodAzureCLI:
		cred, err = azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{
			TenantID: tenantId,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to new Azure CLI credential: %v", err)
		}
		return cred, clientOpt, nil
	case authMethodDefault:
		opt := &azidentity.DefaultAzureCredentialOptions{
			ClientOptions: clientOpt.ClientOptions,
			TenantID:      tenantId,
		}
		cred, err := azidentity.NewDefaultAzureCredential(opt)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to new Default credential: %v", err)
		}
		return cred, clientOpt, nil
	default:
		return nil, nil, fmt.Errorf("unknown auth method: %v", authMethod)
	}
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

func realMain(ctx context.Context, cfg config.Config, batch, mockMeta, plainUI, genMapFile bool, profileType string, effectiveCLI string) (result error) {
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
			log.Printf("[INFO] aztfexport ends")
			tc.Trace(telemetry.Info, "aztfexport ends")
		} else {
			log.Printf("[ERROR] aztfexport ends with error: %v", result)
			tc.Trace(telemetry.Error, fmt.Sprintf("aztfexport ends with error"))
			tc.Trace(telemetry.Error, fmt.Sprintf("Error detail: %v", result))
		}
		tc.Close()
	}()

	log.Printf("[INFO] aztfexport starts with config: %#v", cfg)
	tc.Trace(telemetry.Info, "aztfexport starts")
	tc.Trace(telemetry.Info, "Effective CLI: "+effectiveCLI)

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
