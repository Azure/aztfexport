package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

	"github.com/magodo/armid"
	"github.com/magodo/slog2hclog"
	"github.com/magodo/terraform-client-go/tfclient"
	"github.com/magodo/tfadd/providers/azapi"
	"github.com/magodo/tfadd/providers/azurerm"

	"github.com/Azure/aztfexport/internal"
	"github.com/Azure/aztfexport/internal/ui"
	"github.com/urfave/cli/v2"
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
		if id, err := cfgfile.GetInstallationIdFromPWSH(); err == nil {
			return id, nil
		}
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
			Name:        "subscription-id",
			EnvVars:     []string{"AZTFEXPORT_SUBSCRIPTION_ID", "ARM_SUBSCRIPTION_ID"},
			Aliases:     []string{"s"},
			Usage:       "The subscription id",
			Destination: &flagset.flagSubscriptionId,
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
			Usage:       "Proceed with non-empty output directory, which is likely to pollute the directory and cause errors (use with caution)",
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
			Usage:       fmt.Sprintf("Use the local development provider, instead of the version pinned provider"),
			Destination: &flagset.flagDevProvider,
		},
		&cli.StringFlag{
			Name:        "provider-version",
			EnvVars:     []string{"AZTFEXPORT_PROVIDER_VERSION"},
			Usage:       fmt.Sprintf("The provider version to use for importing. Defaults to %q for azurerm, %s for azapi", azurerm.ProviderSchemaInfo.Version, azapi.ProviderSchemaInfo.Version),
			Destination: &flagset.flagProviderVersion,
		},
		&cli.StringFlag{
			Name:        "provider-name",
			EnvVars:     []string{"AZTFEXPORT_PROVIDER_NAME"},
			Usage:       fmt.Sprintf(`The provider name to use for importing. Possible values are "azurerm" and "azapi". Defaults to "azurerm"`),
			Value:       "azurerm",
			Destination: &flagset.flagProviderName,
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
		&cli.BoolFlag{
			Name:        "mask-sensitive",
			EnvVars:     []string{"AZTFEXPORT_MASK_SENSITIVE"},
			Usage:       "Mask sensitive attributes in the Terraform configuration. This may require manual modifications to produce a valid config",
			Value:       false,
			Destination: &flagset.flagMaskSensitive,
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
		&cli.BoolFlag{
			Name:        "generate-import-block",
			EnvVars:     []string{"AZTFEXPORT_GENERATE_IMPORT_BLOCK"},
			Usage:       `Whether to generate the import.tf that contains the "import" blocks for the Terraform official plannable importing`,
			Destination: &flagset.flagGenerateImportBlock,
		},
		&cli.StringFlag{
			Name:        "log-path",
			EnvVars:     []string{"AZTFEXPORT_LOG_PATH"},
			Usage:       "The file path to store the log",
			Destination: &flagset.flagLogPath,
		},
		&cli.StringFlag{
			Name:        "log-level",
			EnvVars:     []string{"AZTFEXPORT_LOG_LEVEL"},
			Usage:       `Log level, can be one of "ERROR", "WARN", "INFO", "DEBUG" and "TRACE"`,
			Destination: &flagset.flagLogLevel,
			Value:       "INFO",
		},
		&cli.StringSliceFlag{
			Name:        "exclude-azure-resource",
			EnvVars:     []string{"AZTFEXPORT_AZURE_RESOURCE"},
			Usage:       "Exclude resource from being exported based on the Azure resource ID pattern (case-insensitive regexp)",
			Destination: &flagset.flagExcludeAzureResource,
		},
		&cli.StringFlag{
			Name:        "exclude-azure-resource-file",
			EnvVars:     []string{"AZTFEXPORT_AZURE_RESOURCE_FILE"},
			Usage:       "Path to a file recording the excluded resources from being exported based on the Azure resource ID pattern (case-insensitive regexp), one per line",
			Destination: &flagset.flagExcludeAzureResourceFile,
		},
		&cli.StringSliceFlag{
			Name:        "exclude-terraform-resource",
			EnvVars:     []string{"AZTFEXPORT_TERRAFORM_RESOURCE"},
			Usage:       "Exclude resource from being exported based on the Terraform resource type",
			Destination: &flagset.flagExcludeTerraformResource,
		},
		&cli.StringFlag{
			Name:        "exclude-terraform-resource-file",
			EnvVars:     []string{"AZTFEXPORT_TERRAFORM_RESOURCE_FILE"},
			Usage:       "Path to a file recording the excluded resources from being exported based on the Terraform resource type, one per line",
			Destination: &flagset.flagExcludeTerraformResourceFile,
		},
		&cli.BoolFlag{
			Name:        "include-role-assignment",
			EnvVars:     []string{"AZTFEXPORT_INCLUDE_ROLE_ASSIGNMENT"},
			Usage:       `Whether to include role assignments assigned to the resources exported`,
			Destination: &flagset.flagIncludeRoleAssignment,
		},

		// Common flags (auth)
		&cli.StringFlag{
			Name:        "env",
			EnvVars:     []string{"AZTFEXPORT_ENV", "ARM_ENVIRONMENT"},
			Usage:       `The cloud environment, can be one of "public", "usgovernment" and "china"`,
			Destination: &flagset.flagEnv,
			Value:       "public",
		},
		&cli.StringFlag{
			Name:        "tenant-id",
			EnvVars:     []string{"AZTFEXPORT_TENANT_ID", "ARM_TENANT_ID"},
			Usage:       "The Azure tenant id",
			Destination: &flagset.flagTenantId,
		},
		&cli.StringSliceFlag{
			Name:        "auxiliary-tenant-ids",
			EnvVars:     []string{"AZTFEXPORT_AUXILIARY_TENANT_IDS", "ARM_AUXILIARY_TENANT_IDS"},
			Usage:       "The Azure auxiliary tenant ids (comma separated)",
			Destination: &flagset.flagAuxiliaryTenantIds,
		},
		&cli.StringFlag{
			Name:        "client-id",
			EnvVars:     []string{"AZTFEXPORT_CLIENT_ID", "ARM_CLIENT_ID"},
			Usage:       "The Azure client id",
			Destination: &flagset.flagClientId,
		},
		&cli.StringFlag{
			Name:        "client-id-file-path",
			EnvVars:     []string{"AZTFEXPORT_CLIENT_ID_FILE_PATH", "ARM_CLIENT_ID_FILE_PATH"},
			Usage:       "The Azure client id file path",
			Destination: &flagset.flagClientIdFilePath,
		},
		&cli.StringFlag{
			Name:        "client-certificate",
			EnvVars:     []string{"AZTFEXPORT_CLIENT_CERTIFICATE", "ARM_CLIENT_CERTIFICATE"},
			Usage:       "A base64-encoded PKCS#12 bundle to be used as the client certificate for authentication",
			Destination: &flagset.flagClientCertificate,
		},
		&cli.StringFlag{
			Name:        "client-certificate-path",
			EnvVars:     []string{"AZTFEXPORT_CLIENT_CERTIFICATE_PATH", "ARM_CLIENT_CERTIFICATE_PATH"},
			Usage:       "The path to the Client Certificate (PKCS#12) associated with the Service Principal which should be used",
			Destination: &flagset.flagClientCertificatePath,
		},
		&cli.StringFlag{
			Name:        "client-certificate-password",
			EnvVars:     []string{"AZTFEXPORT_CLIENT_CERTIFICATE_PASSWORD", "ARM_CLIENT_CERTIFICATE_PASSWORD"},
			Usage:       "The password associated with the Client Certificate",
			Destination: &flagset.flagClientCertificatePassword,
		},
		&cli.StringFlag{
			Name:        "client-secret",
			EnvVars:     []string{"AZTFEXPORT_CLIENT_SECRET", "ARM_CLIENT_SECRET"},
			Usage:       "The Azure client secret",
			Destination: &flagset.flagClientSecret,
		},
		&cli.StringFlag{
			Name:        "client-secret-file-path",
			EnvVars:     []string{"AZTFEXPORT_CLIENT_SECRET_FILE_PATH", "ARM_CLIENT_SECRET_FILE_PATH"},
			Usage:       "The Azure client secret file path",
			Destination: &flagset.flagClientSecretFilePath,
		},
		&cli.StringFlag{
			Name:        "oidc-request-token",
			EnvVars:     []string{"AZTFEXPORT_OIDC_REQUEST_TOKEN", "ARM_OIDC_REQUEST_TOKEN", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"},
			Usage:       "The bearer token for the request to the OIDC provider",
			Destination: &flagset.flagOIDCRequestToken,
		},
		&cli.StringFlag{
			Name:        "oidc-request-url",
			EnvVars:     []string{"AZTFEXPORT_OIDC_REQUEST_URL", "ARM_OIDC_REQUEST_URL", "ACTIONS_ID_TOKEN_REQUEST_URL"},
			Usage:       "The URL for the OIDC provider from which to request an ID token",
			Destination: &flagset.flagOIDCRequestURL,
		},
		&cli.StringFlag{
			Name:        "oidc-token-file-path",
			EnvVars:     []string{"AZTFEXPORT_OIDC_TOKEN_FILE_PATH", "ARM_OIDC_TOKEN_FILE_PATH"},
			Usage:       "The path to a file containing an ID token when authenticating using OIDC",
			Destination: &flagset.flagOIDCTokenFilePath,
		},
		&cli.StringFlag{
			Name:        "oidc-token",
			EnvVars:     []string{"AZTFEXPORT_OIDC_TOKEN", "ARM_OIDC_TOKEN"},
			Usage:       "The ID token when authenticating using OIDC",
			Destination: &flagset.flagOIDCToken,
		},
		&cli.BoolFlag{
			Name:        "use-managed-identity-cred",
			EnvVars:     []string{"AZTFEXPORT_USE_MANAGED_IDENTITY_CRED", "ARM_USE_MSI"},
			Usage:       "Explicitly use the managed identity that is provided by the Azure host to do authentication",
			Destination: &flagset.flagUseManagedIdentityCred,
			Value:       false,
		},
		&cli.BoolFlag{
			Name:        "use-azure-cli-cred",
			EnvVars:     []string{"AZTFEXPORT_USE_AZURE_CLI_CRED", "ARM_USE_CLI"},
			Usage:       "Explicitly use the Azure CLI to do authentication",
			Destination: &flagset.flagUseAzureCLICred,
			Value:       true,
		},
		&cli.BoolFlag{
			Name:        "use-oidc-cred",
			EnvVars:     []string{"AZTFEXPORT_USE_OIDC_CRED", "ARM_USE_OIDC"},
			Usage:       "Explicitly use the OIDC to do authentication",
			Destination: &flagset.flagUseOIDCCred,
			Value:       false,
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
			Usage:       `The Terraform resource name (only works for single resource mode).`,
			Destination: &flagset.flagResName,
		},
		&cli.StringFlag{
			Name:        "type",
			EnvVars:     []string{"AZTFEXPORT_TYPE"},
			Usage:       `The Terraform resource type (only works for single resource mode).`,
			Destination: &flagset.flagResType,
		},
		&cli.StringFlag{
			Name:        "name-pattern",
			EnvVars:     []string{"AZTFEXPORT_NAME_PATTERN"},
			Aliases:     []string{"p"},
			Usage:       `The pattern of the resource name. The semantic of a pattern is the same as Go's os.CreateTemp() (only works for multi-resource mode).`,
			Value:       "res-",
			Destination: &flagset.flagPattern,
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
		&cli.BoolFlag{
			Name:        "include-resource-group",
			EnvVars:     []string{"AZTFEXPORT_INCLUDE_RESOURCE_GROUP"},
			Usage:       "Include the resource groups that the exported resources belong to",
			Destination: &flagset.flagIncludeResourceGroup,
		},
		&cli.StringFlag{
			Name:        "arg-table",
			EnvVars:     []string{"AZTFEXPORT_ARG_TABLE"},
			Usage:       `The Azure Resource Graph table name. Defaults to "Resources".`,
			Destination: &flagset.flagARGTable,
		},
		&cli.StringFlag{
			Name:        "arg-authorization-scope-filter",
			EnvVars:     []string{"AZTFEXPORT_ARG_AUTHORIZATION_SCOPE_FILTER"},
			Usage:       `The Azure Resource Graph Authorization Scope Filter parameter. Possible values are: "AtScopeAndBelow", "AtScopeAndAbove", "AtScopeAboveAndBelow" and "AtScopeExact"`,
			Destination: &flagset.flagARGAuthorizationScopeFilter,
		},
	}, resourceGroupFlags...)

	mappingFileFlags := append([]cli.Flag{}, commonFlags...)

	app := &cli.App{
		Name:      "aztfexport",
		Version:   getVersion(),
		Usage:     "A tool to bring existing Azure resources under Terraform's management",
		UsageText: "aztfexport <command> [option] <scope>",
		Before:    prepareConfigFile,
		Commands: []*cli.Command{
			{
				Name:      "config",
				Usage:     `Configuring the tool.`,
				UsageText: "aztfexport config [subcommand]",
				Subcommands: []*cli.Command{
					{
						Name:      "set",
						Usage:     `Set a configuration item for aztfexport.`,
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
						Usage:     `Get a configuration item for aztfexport.`,
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
						Usage:     `Show the full configuration for aztfexport.`,
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
				Name:      string(ModeResource),
				Aliases:   []string{"res"},
				Usage:     "Exporting one or more resources. The arguments can be resource ids, or path to files (prefixed with `@`) that contain resource id in each line.",
				UsageText: "aztfexport resource [option] [<resourceId> | @<resourceIdFile>...]",
				Flags:     resourceFlags,
				Before:    commandBeforeFunc(&flagset, ModeResource),
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No resource id specified")
					}

					var resIds []string
					for _, arg := range c.Args().Slice() {
						if !strings.HasPrefix(arg, "@") {
							if _, err := armid.ParseResourceId(arg); err != nil {
								return fmt.Errorf("invalid resource id: %v", err)
							}
							resIds = append(resIds, arg)
							continue
						}

						path := strings.TrimPrefix(arg, "@")
						// #nosec G304
						f, err := os.Open(path)
						if err != nil {
							return fmt.Errorf("failed to open file %q: %v", path, err)
						}

						if err := func() error {
							defer f.Close()
							scanner := bufio.NewScanner(f)
							for scanner.Scan() {
								resId := strings.TrimSpace(scanner.Text())
								if _, err := armid.ParseResourceId(resId); err != nil {
									return fmt.Errorf("invalid resource id (contained in %q): %v", path, err)
								}
								resIds = append(resIds, resId)
							}
							return scanner.Err()
						}(); err != nil {
							return fmt.Errorf("scanning %q: %v", path, err)
						}
					}

					commonConfig, err := flagset.BuildCommonConfig()
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig:          commonConfig,
						ResourceIds:           resIds,
						TFResourceName:        flagset.flagResName,
						TFResourceType:        flagset.flagResType,
						ResourceNamePattern:   flagset.flagPattern,
						IncludeRoleAssignment: flagset.flagIncludeRoleAssignment,
					}

					return realMain(c.Context, cfg, flagset.flagNonInteractive, flagset.hflagMockClient, flagset.flagPlainUI, flagset.flagGenerateMappingFile, flagset.hflagProfile, flagset.DescribeCLI(ModeResource), flagset.hflagTFClientPluginPath)
				},
			},
			{
				Name:      string(ModeResourceGroup),
				Aliases:   []string{"rg"},
				Usage:     "Exporting a resource group and the nested resources resides within it.",
				UsageText: "aztfexport resource-group [option] <resource group name>",
				Flags:     resourceGroupFlags,
				Before:    commandBeforeFunc(&flagset, ModeResourceGroup),
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No resource group specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one resource groups specified")
					}

					rg := c.Args().First()

					commonConfig, err := flagset.BuildCommonConfig()
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig:          commonConfig,
						ResourceGroupName:     rg,
						ResourceNamePattern:   flagset.flagPattern,
						RecursiveQuery:        true,
						IncludeRoleAssignment: flagset.flagIncludeRoleAssignment,
					}

					return realMain(c.Context, cfg, flagset.flagNonInteractive, flagset.hflagMockClient, flagset.flagPlainUI, flagset.flagGenerateMappingFile, flagset.hflagProfile, flagset.DescribeCLI(ModeResourceGroup), flagset.hflagTFClientPluginPath)
				},
			},
			{
				Name:      string(ModeQuery),
				Usage:     "Exporting a customized scope of resources determined by an Azure Resource Graph where predicate.",
				UsageText: "aztfexport query [option] <ARG where predicate>",
				Flags:     queryFlags,
				Before:    commandBeforeFunc(&flagset, ModeQuery),
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No query specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one queries specified. Use `and` with double quotes to run multiple query parameters.")
					}

					predicate := c.Args().First()

					commonConfig, err := flagset.BuildCommonConfig()
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig:                commonConfig,
						ARGPredicate:                predicate,
						ResourceNamePattern:         flagset.flagPattern,
						RecursiveQuery:              flagset.flagRecursive,
						IncludeRoleAssignment:       flagset.flagIncludeRoleAssignment,
						IncludeResourceGroup:        flagset.flagIncludeResourceGroup,
						ARGTable:                    flagset.flagARGTable,
						ARGAuthorizationScopeFilter: flagset.flagARGAuthorizationScopeFilter,
					}

					return realMain(c.Context, cfg, flagset.flagNonInteractive, flagset.hflagMockClient, flagset.flagPlainUI, flagset.flagGenerateMappingFile, flagset.hflagProfile, flagset.DescribeCLI(ModeQuery), flagset.hflagTFClientPluginPath)
				},
			},
			{
				Name:      string(ModeMappingFile),
				Aliases:   []string{"map"},
				Usage:     "Exporting a customized scope of resources determined by the resource mapping file.",
				UsageText: "aztfexport mapping-file [option] <resource mapping file>",
				Flags:     mappingFileFlags,
				Before:    commandBeforeFunc(&flagset, ModeMappingFile),
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("No resource mapping file specified")
					}
					if c.NArg() > 1 {
						return fmt.Errorf("More than one resource mapping files specified")
					}

					mapFile := c.Args().First()

					commonConfig, err := flagset.BuildCommonConfig()
					if err != nil {
						return err
					}

					// Initialize the config
					cfg := config.Config{
						CommonConfig: commonConfig,
						MappingFile:  mapFile,
					}

					return realMain(c.Context, cfg, flagset.flagNonInteractive, flagset.hflagMockClient, flagset.flagPlainUI, flagset.flagGenerateMappingFile, flagset.hflagProfile, flagset.DescribeCLI(ModeMappingFile), flagset.hflagTFClientPluginPath)
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

func realMain(ctx context.Context, cfg config.Config, batch, mockMeta, plainUI, genMapFile bool, profileType string, effectiveCLI string, tfClientPluginPath string) (result error) {
	switch strings.ToLower(profileType) {
	case "cpu":
		defer profile.Start(profile.CPUProfile, profile.ProfilePath("."), profile.NoShutdownHook).Stop()
	case "mem":
		defer profile.Start(profile.MemProfile, profile.ProfilePath("."), profile.NoShutdownHook).Stop()
	}

	// Initialize the TFClient
	if tfClientPluginPath != "" {
		// #nosec G204
		cmd := exec.Command(flagset.hflagTFClientPluginPath)
		cmd.Env = append(cmd.Env,
			// Disable AzureRM provider's enahnced validation, which will cause RP listing, that is expensive.
			// The setting for with_tf version is done during the init_tf function of meta Init phase.
			"ARM_PROVIDER_ENHANCED_VALIDATION=false",
			// AzureRM provider will honor env.var "AZURE_HTTP_USER_AGENT" when constructing for HTTP "User-Agent" header.
			// The setting for with_tf version is done during the init_tf function of meta Init phase.
			"AZURE_HTTP_USER_AGENT="+cfg.AzureSDKClientOption.Telemetry.ApplicationID,
		)
		tfc, err := tfclient.New(tfclient.Option{
			Cmd:    cmd,
			Logger: slog2hclog.New(cfg.Logger.WithGroup("provider"), nil),
		})
		if err != nil {
			return err
		}
		cfg.TFClient = tfc
	}

	tc := cfg.TelemetryClient

	defer func() {
		if result == nil {
			cfg.Logger.Info("aztfexport ends")
			tc.Trace(telemetry.Info, "aztfexport ends")
		} else {
			cfg.Logger.Error("aztfexport ends with error", "error", result)
			tc.Trace(telemetry.Error, fmt.Sprintf("aztfexport ends with error"))
			tc.Trace(telemetry.Error, fmt.Sprintf("Error detail: %v", result))
		}
		tc.Close()
	}()

	cfg.Logger.Info("aztfexport starts", "config", fmt.Sprintf("%#v", cfg))
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
