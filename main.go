package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/magodo/armid"
	"github.com/magodo/tfadd/providers/azurerm"

	"github.com/Azure/aztfy/internal"
	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/ui"
	azlog "github.com/Azure/azure-sdk-for-go/sdk/azcore/log"
	"github.com/urfave/cli/v2"
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

		// common flags (hidden)
		hflagMockClient bool
		hflagLogPath    string
		hflagPlainUI    bool

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

	beforeFunc := func(ctx *cli.Context) error {
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
			Usage:   "The output directory",
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

		// Hidden flags
		&cli.BoolFlag{
			Name:        "mock-client",
			EnvVars:     []string{"AZTFY_MOCK_CLIENT"},
			Usage:       "Whether to mock the client. This is for testing UI",
			Hidden:      true,
			Destination: &hflagMockClient,
		},
		&cli.StringFlag{
			Name:        "log-path",
			EnvVars:     []string{"AZTFY_LOG_PATH"},
			Usage:       "The path to store the log",
			Hidden:      true,
			Destination: &hflagLogPath,
		},

		&cli.BoolFlag{
			Name:        "plain-ui",
			EnvVars:     []string{"AZTFY_PLAIN_UI"},
			Usage:       "In non-interactive mode, print the progress information line by line, rather than the spinner UI",
			Hidden:      true,
			Destination: &hflagPlainUI,
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

	app := &cli.App{
		Name:      "aztfy",
		Version:   getVersion(),
		Usage:     "Bring existing Azure resources under Terraform's management",
		UsageText: "aztfy [command] [option]",
		Commands: []*cli.Command{
			{
				Name:      "resource",
				Aliases:   []string{"res"},
				Usage:     "Terrafying a single resource",
				UsageText: "aztfy resource [option] <resource id>",
				Flags:     resourceFlags,
				Before:    beforeFunc,
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

					// Initialize the config
					cfg := config.Config{
						MockClient: hflagMockClient,
						CommonConfig: config.CommonConfig{
							LogPath:             hflagLogPath,
							SubscriptionId:      flagSubscriptionId,
							OutputDir:           flagOutputDir,
							Overwrite:           flagOverwrite,
							Append:              flagAppend,
							DevProvider:         flagDevProvider,
							Batch:               flagNonInteractive,
							ContinueOnError:     flagContinue,
							BackendType:         flagBackendType,
							BackendConfig:       flagBackendConfig.Value(),
							FullConfig:          flagFullConfig,
							Parallelism:         flagParallelism,
							PlainUI:             hflagPlainUI,
							GenerateMappingFile: flagGenerateMappingFile,
							HCLOnly:             flagHCLOnly,
						},
						ResourceId:     resId,
						TFResourceName: flagResName,
						TFResourceType: flagResType,
					}

					return realMain(cfg)
				},
			},
			{
				Name:      "resource-group",
				Aliases:   []string{"rg"},
				Usage:     "Terrafying a resource group and the nested resources resides within it",
				UsageText: "aztfy resource-group [option] <resource group name>",
				Flags:     resourceGroupFlags,
				Before:    beforeFunc,
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No resource group specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one resource groups specified")
					}

					rg := c.Args().First()

					// Initialize the config
					cfg := config.Config{
						MockClient: hflagMockClient,
						CommonConfig: config.CommonConfig{
							LogPath:             hflagLogPath,
							SubscriptionId:      flagSubscriptionId,
							OutputDir:           flagOutputDir,
							Overwrite:           flagOverwrite,
							Append:              flagAppend,
							DevProvider:         flagDevProvider,
							Batch:               flagNonInteractive,
							ContinueOnError:     flagContinue,
							BackendType:         flagBackendType,
							BackendConfig:       flagBackendConfig.Value(),
							FullConfig:          flagFullConfig,
							Parallelism:         flagParallelism,
							PlainUI:             hflagPlainUI,
							GenerateMappingFile: flagGenerateMappingFile,
							HCLOnly:             flagHCLOnly,
						},
						ResourceGroupName:   rg,
						ResourceNamePattern: flagPattern,
						RecursiveQuery:      true,
					}

					return realMain(cfg)
				},
			},
			{
				Name:      "query",
				Usage:     "Terrafying a customized scope of resources determined by an Azure Resource Graph where predicate",
				UsageText: "aztfy query [option] <ARG where predicate>",
				Flags:     queryFlags,
				Before:    beforeFunc,
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No query specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one queries specified")
					}

					predicate := c.Args().First()

					// Initialize the config
					cfg := config.Config{
						MockClient: hflagMockClient,
						CommonConfig: config.CommonConfig{
							LogPath:             hflagLogPath,
							SubscriptionId:      flagSubscriptionId,
							OutputDir:           flagOutputDir,
							Overwrite:           flagOverwrite,
							Append:              flagAppend,
							DevProvider:         flagDevProvider,
							Batch:               flagNonInteractive,
							ContinueOnError:     flagContinue,
							BackendType:         flagBackendType,
							BackendConfig:       flagBackendConfig.Value(),
							FullConfig:          flagFullConfig,
							Parallelism:         flagParallelism,
							PlainUI:             hflagPlainUI,
							GenerateMappingFile: flagGenerateMappingFile,
							HCLOnly:             flagHCLOnly,
						},
						ARGPredicate:        predicate,
						ResourceNamePattern: flagPattern,
						RecursiveQuery:      flagRecursive,
					}

					return realMain(cfg)
				},
			},
			{
				Name:      "mapping-file",
				Aliases:   []string{"map"},
				Usage:     "Terrafying a customized scope of resources determined by the resource mapping file",
				UsageText: "aztfy mapping-file [option] <resource mapping file>",
				Flags:     mappingFileFlags,
				Before:    beforeFunc,
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No resource mapping file specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one resource mapping files specified")
					}

					mapFile := c.Args().First()

					// Initialize the config
					cfg := config.Config{
						MockClient: hflagMockClient,
						CommonConfig: config.CommonConfig{
							SubscriptionId:      flagSubscriptionId,
							OutputDir:           flagOutputDir,
							Overwrite:           flagOverwrite,
							Append:              flagAppend,
							DevProvider:         flagDevProvider,
							Batch:               flagNonInteractive,
							ContinueOnError:     flagContinue,
							BackendType:         flagBackendType,
							BackendConfig:       flagBackendConfig.Value(),
							FullConfig:          flagFullConfig,
							Parallelism:         flagParallelism,
							PlainUI:             hflagPlainUI,
							GenerateMappingFile: flagGenerateMappingFile,
							HCLOnly:             flagHCLOnly,
						},
						MappingFile: mapFile,
					}

					return realMain(cfg)
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

func initLog(path string) error {
	log.SetOutput(io.Discard)
	if path != "" {
		log.SetPrefix("[aztfy] ")
		// #nosec G304
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return fmt.Errorf("creating log file %s: %v", path, err)
		}
		log.SetOutput(f)

		// Enable the logging for the Azure SDK
		os.Setenv("AZURE_SDK_GO_LOGGING", "all") // #nosec G104
		azlog.SetListener(func(cls azlog.Event, msg string) {
			log.Printf("[SDK] %s: %s\n", cls, msg)
		})
	}
	return nil
}

func subscriptionIdFromCLI() (string, error) {
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd := exec.Command("az", "account", "show", "--query", "id")
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

func realMain(cfg config.Config) error {
	// Initialize log
	if err := initLog(cfg.LogPath); err != nil {
		return err
	}

	// Run in non-interactive mode
	if cfg.Batch {
		if err := internal.BatchImport(cfg); err != nil {
			return err
		}
		return nil
	}

	// Run in interactive mode
	prog, err := ui.NewProgram(cfg)
	if err != nil {
		return err
	}
	if err := prog.Start(); err != nil {
		return err
	}
	return nil
}
