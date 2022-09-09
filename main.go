package main

import (
	"bytes"
	"encoding/json"
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
		flagSubscriptionId string
		flagOutputDir      string
		flagOverwrite      bool
		flagAppend         bool
		flagDevProvider    bool
		flagBackendType    string
		flagBackendConfig  cli.StringSlice
		flagFullConfig     bool

		// common flags (hidden)
		hflagLogPath string

		// rg-only flags
		flagBatchMode   bool
		flagContinue    bool
		flagMappingFile string
		flagPattern     string

		// rg-only flags (hidden)
		hflagMockClient bool

		// res-only flags
		flagName    string
		flagResType string
	)

	commonFlagsCheck := func() error {
		if flagAppend {
			if flagBackendType != "local" {
				return fmt.Errorf("`--append` only works for local backend")
			}
			if flagOverwrite {
				return fmt.Errorf("`--append` conflicts with `--overwrite`")
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

		// Hidden flags
		&cli.StringFlag{
			Name:        "log-path",
			EnvVars:     []string{"AZTFY_LOG_PATH"},
			Usage:       "The path to store the log",
			Hidden:      true,
			Destination: &hflagLogPath,
		},
	}

	app := &cli.App{
		Name:      "aztfy",
		Version:   getVersion(),
		Usage:     "Bring existing Azure resources under Terraform's management",
		UsageText: "aztfy [command] [option]",
		Commands: []*cli.Command{
			{
				Name:      "resource-group",
				Aliases:   []string{"rg"},
				Usage:     "Terrafying a resource group and the nested resources resides within it",
				UsageText: "aztfy resource-group [option] <resource group name>",
				Flags: append([]cli.Flag{
					&cli.BoolFlag{
						Name:        "batch",
						EnvVars:     []string{"AZTFY_BATCH"},
						Aliases:     []string{"b"},
						Usage:       "Batch mode (i.e. Non-interactive mode)",
						Destination: &flagBatchMode,
					},
					&cli.StringFlag{
						Name:        "resource-mapping",
						EnvVars:     []string{"AZTFY_RESOURCE_MAPPING"},
						Aliases:     []string{"m"},
						Usage:       "The resource mapping file",
						Destination: &flagMappingFile,
					},
					&cli.BoolFlag{
						Name:        "continue",
						EnvVars:     []string{"AZTFY_CONTINUE"},
						Aliases:     []string{"k"},
						Usage:       "Whether continue on import error (batch mode only)",
						Destination: &flagContinue,
					},
					&cli.StringFlag{
						Name:        "name-pattern",
						EnvVars:     []string{"AZTFY_NAME_PATTERN"},
						Aliases:     []string{"p"},
						Usage:       `The pattern of the resource name. The semantic of a pattern is the same as Go's os.CreateTemp()`,
						Value:       "res-",
						Destination: &flagPattern,
					},

					// Hidden flags
					&cli.BoolFlag{
						Name:        "mock-client",
						EnvVars:     []string{"AZTFY_MOCK_CLIENT"},
						Usage:       "Whether to mock the client. This is for testing UI",
						Hidden:      true,
						Destination: &hflagMockClient,
					},
				}, commonFlags...),
				Action: func(c *cli.Context) error {
					if err := commonFlagsCheck(); err != nil {
						return err
					}
					if c.NArg() == 0 {
						return fmt.Errorf("No resource group specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one resource groups specified")
					}
					if flagContinue && !flagBatchMode {
						return fmt.Errorf("`--continue` must be used together with `--batch`")
					}

					rg := c.Args().First()

					// Initialize log
					if err := initLog(hflagLogPath); err != nil {
						return err
					}

					// Identify the subscription id, which comes from one of following (starts from the highest priority):
					// - Command line option
					// - Env variable: AZTFY_SUBSCRIPTION_ID
					// - Env variable: ARM_SUBSCRIPTION_ID
					// - Output of azure cli, the current active subscription
					subscriptionId := flagSubscriptionId
					if subscriptionId == "" {
						var err error
						subscriptionId, err = subscriptionIdFromCLI()
						if err != nil {
							return fmt.Errorf("retrieving subscription id from CLI: %v", err)
						}
					}

					// Initialize the config
					cfg := config.RgConfig{
						MockClient: hflagMockClient,
						CommonConfig: config.CommonConfig{
							SubscriptionId: subscriptionId,
							OutputDir:      flagOutputDir,
							Overwrite:      flagOverwrite,
							Append:         flagAppend,
							DevProvider:    flagDevProvider,
							BackendType:    flagBackendType,
							BackendConfig:  flagBackendConfig.Value(),
							FullConfig:     flagFullConfig,
						},
					}

					if flagMappingFile != "" {
						b, err := os.ReadFile(flagMappingFile)
						if err != nil {
							return fmt.Errorf("reading mapping file %s: %v", flagMappingFile, err)
						}
						if err := json.Unmarshal(b, &cfg.ResourceMapping); err != nil {
							return fmt.Errorf("unmarshalling the mapping file: %v", err)
						}
					}
					cfg.ResourceGroupName = rg
					cfg.ResourceNamePattern = flagPattern
					cfg.BatchMode = flagBatchMode

					// Run in batch mode
					if cfg.BatchMode {
						if err := internal.BatchImport(cfg, flagContinue); err != nil {
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
				},
			},
			{
				Name:      "resource",
				Aliases:   []string{"res"},
				Usage:     "Terrafying a single resource",
				UsageText: "aztfy resource [option] <resource id>",
				Flags: append([]cli.Flag{
					&cli.StringFlag{
						Name:        "name",
						EnvVars:     []string{"AZTFY_NAME"},
						Aliases:     []string{"n"},
						Usage:       `The Terraform resource name.`,
						Value:       "res-0",
						Destination: &flagName,
					},
					&cli.StringFlag{
						Name:        "type",
						EnvVars:     []string{"AZTFY_TYPE"},
						Usage:       `The Terraform resource type.`,
						Destination: &flagResType,
					},
				}, commonFlags...),
				Action: func(c *cli.Context) error {
					if err := commonFlagsCheck(); err != nil {
						return err
					}
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

					// Initialize log
					if err := initLog(hflagLogPath); err != nil {
						return err
					}

					// Identify the subscription id, which comes from one of following (starts from the highest priority):
					// - Command line option
					// - Env variable: AZTFY_SUBSCRIPTION_ID
					// - Env variable: ARM_SUBSCRIPTION_ID
					// - Output of azure cli, the current active subscription
					subscriptionId := flagSubscriptionId
					if subscriptionId == "" {
						var err error
						subscriptionId, err = subscriptionIdFromCLI()
						if err != nil {
							return fmt.Errorf("retrieving subscription id from CLI: %v", err)
						}
					}

					// Initialize the config
					cfg := config.ResConfig{
						CommonConfig: config.CommonConfig{
							SubscriptionId: subscriptionId,
							OutputDir:      flagOutputDir,
							Overwrite:      flagOverwrite,
							Append:         flagAppend,
							DevProvider:    flagDevProvider,
							BatchMode:      true,
							BackendType:    flagBackendType,
							BackendConfig:  flagBackendConfig.Value(),
							FullConfig:     flagFullConfig,
						},
						ResourceId:   resId,
						ResourceName: flagName,
						ResourceType: flagResType,
					}

					return internal.ResourceImport(cfg)
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
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("creating log file %s: %v", path, err)
		}
		log.SetOutput(f)

		// Enable the logging for the Azure SDK
		os.Setenv("AZURE_SDK_GO_LOGGING", "all")
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
