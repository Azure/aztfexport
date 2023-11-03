package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Azure/aztfexport/internal/cfgfile"
	"github.com/Azure/aztfexport/pkg/config"
	"github.com/Azure/aztfexport/pkg/telemetry"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/gofrs/uuid"
	"github.com/hashicorp/go-hclog"
	"github.com/magodo/terraform-client-go/tfclient"
	"github.com/urfave/cli/v2"
)

var flagset FlagSet

type FlagSet struct {
	// common flags
	flagEnv                 string
	flagSubscriptionId      string
	flagOutputDir           string
	flagOverwrite           bool
	flagAppend              bool
	flagDevProvider         bool
	flagProviderVersion     string
	flagProviderName        string
	flagBackendType         string
	flagBackendConfig       cli.StringSlice
	flagFullConfig          bool
	flagParallelism         int
	flagContinue            bool
	flagNonInteractive      bool
	flagPlainUI             bool
	flagGenerateMappingFile bool
	flagHCLOnly             bool
	flagModulePath          string

	// common flags (auth)
	flagUseEnvironmentCred     bool
	flagUseManagedIdentityCred bool
	flagUseAzureCLICred        bool
	flagUseOIDCCred            bool
	flagOIDCRequestToken       string
	flagOIDCRequestURL         string
	flagOIDCTokenFilePath      string
	flagOIDCToken              string

	// common flags (hidden)
	hflagMockClient         bool
	hflagProfile            string
	hflagTFClientPluginPath string

	// Subcommand specific flags
	//
	// res:
	// flagResName
	// flagResType
	//
	// rg:
	// flagPattern
	// flagIncludeRoleAssignment
	//
	// query:
	// flagPattern
	// flagRecursive
	// flagIncludeRoleAssignment
	// flagIncludeResourceGroup
	flagPattern               string
	flagRecursive             bool
	flagResName               string
	flagResType               string
	flagIncludeRoleAssignment bool
	flagIncludeResourceGroup  bool
}

const (
	ModeResource      = "resource"
	ModeResourceGroup = "resource-group"
	ModeQuery         = "query"
	ModeMappingFile   = "mapping-file"
)

// DescribeCLI construct a description of the CLI based on the flag set and the specified mode.
// The main reason is to record the usage of some "interesting" options in the telemetry.
// Note that only insensitive values are recorded (i.e. subscription id, resource id, etc are not recorded)
func (flag FlagSet) DescribeCLI(mode string) string {
	args := []string{mode}

	// The following flags are skipped eiter not interesting, or might contain sensitive info:
	// - flagSubscriptionId
	// - flagOutputDir
	// - flagDevProvider
	// - flagBackendConfig
	// - all hflags

	if flag.flagEnv != "" {
		args = append(args, "--env="+flag.flagEnv)
	}
	if flag.flagOverwrite {
		args = append(args, "--overwrite=true")
	}
	if flag.flagAppend {
		args = append(args, "--append=true")
	}
	if flag.flagProviderVersion != "" {
		args = append(args, `-provider-version="%s"`, flag.flagProviderVersion)
	}
	if flag.flagProviderName != "" {
		args = append(args, `-provider-name="%s"`, flag.flagProviderName)
	}
	if flag.flagBackendType != "" {
		args = append(args, "--backend-type="+flag.flagBackendType)
	}
	if flag.flagFullConfig {
		args = append(args, "--full-properties=true")
	}
	if flag.flagParallelism != 0 {
		args = append(args, fmt.Sprintf("--parallelism=%d", flag.flagParallelism))
	}
	if flag.flagNonInteractive {
		args = append(args, "--non-interactive=true")
	}
	if flag.flagPlainUI {
		args = append(args, "--plain-ui=true")
	}
	if flag.flagContinue {
		args = append(args, "--continue=true")
	}
	if flag.flagGenerateMappingFile {
		args = append(args, "--generate-mapping-file=true")
	}
	if flag.flagHCLOnly {
		args = append(args, "--hcl-only=true")
	}
	if flag.flagModulePath != "" {
		args = append(args, "--module-path="+flag.flagModulePath)
	}

	if flag.flagUseEnvironmentCred {
		args = append(args, "--use-environment-cred=true")
	}
	if flag.flagUseManagedIdentityCred {
		args = append(args, "--use-managed-identity-cred=true")
	}
	if flag.flagUseAzureCLICred {
		args = append(args, "--use-azure-cli-cred=true")
	}
	if flag.flagUseOIDCCred {
		args = append(args, "--use-oidc-cred=true")
	}
	if flag.flagOIDCRequestToken != "" {
		args = append(args, "--oidc-request-token=*")
	}
	if flag.flagOIDCRequestURL != "" {
		args = append(args, "--oidc-request-url="+flag.flagOIDCRequestURL)
	}
	if flag.flagOIDCTokenFilePath != "" {
		args = append(args, "--oidc-token-file-path="+flag.flagOIDCTokenFilePath)
	}
	if flag.flagOIDCToken != "" {
		args = append(args, "--oidc-token=*")
	}

	if flag.hflagTFClientPluginPath != "" {
		args = append(args, "--tfclient-plugin-path="+flag.hflagTFClientPluginPath)
	}
	switch mode {
	case ModeResource:
		if flag.flagResName != "" {
			args = append(args, "--name="+flag.flagResName)
		}
		if flag.flagResType != "" {
			args = append(args, "--type="+flag.flagResType)
		}
	case ModeResourceGroup:
		if flag.flagPattern != "" {
			args = append(args, "--name-pattern="+flag.flagPattern)
		}
		if flag.flagIncludeRoleAssignment {
			args = append(args, "--include-role-assignment=true")
		}
	case ModeQuery:
		if flag.flagPattern != "" {
			args = append(args, "--name-pattern="+flag.flagPattern)
		}
		if flag.flagRecursive {
			args = append(args, "--recursive=true")
		}
		if flag.flagIncludeRoleAssignment {
			args = append(args, "--include-role-assignment=true")
		}
		if flag.flagIncludeResourceGroup {
			args = append(args, "--include-resource-group=true")
		}
	}
	return "aztfexport " + strings.Join(args, " ")
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

// buildAzureSDKCredAndClientOpt builds the Azure SDK credential and client option from multiple sources (i.e. environment variables, MSI, Azure CLI).
func buildAzureSDKCredAndClientOpt(fset FlagSet) (azcore.TokenCredential, *arm.ClientOptions, error) {
	var cloudCfg cloud.Configuration
	switch env := fset.flagEnv; strings.ToLower(env) {
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
				ApplicationID: fmt.Sprintf("aztfexport(%s)", fset.flagProviderName),
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
	switch {
	case fset.flagUseEnvironmentCred:
		cred, err = azidentity.NewEnvironmentCredential(&azidentity.EnvironmentCredentialOptions{
			ClientOptions: clientOpt.ClientOptions,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to new Environment credential: %v", err)
		}
		return cred, clientOpt, nil
	case fset.flagUseManagedIdentityCred:
		cred, err = azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
			ClientOptions: clientOpt.ClientOptions,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to new Managed Identity credential: %v", err)
		}
		return cred, clientOpt, nil
	case fset.flagUseAzureCLICred:
		cred, err = azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{
			TenantID: tenantId,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to new Azure CLI credential: %v", err)
		}
		return cred, clientOpt, nil
	case fset.flagUseOIDCCred:
		cred, err = NewOidcCredential(&OidcCredentialOptions{
			ClientOptions: clientOpt.ClientOptions,
			TenantID:      tenantId,
			ClientID:      os.Getenv("ARM_CLIENT_ID"),
			RequestToken:  fset.flagOIDCRequestToken,
			RequestUrl:    fset.flagOIDCRequestURL,
			Token:         fset.flagOIDCToken,
			TokenFilePath: fset.flagOIDCTokenFilePath,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to new OIDC credential: %v", err)
		}
		return cred, clientOpt, nil
	default:
		opt := &azidentity.DefaultAzureCredentialOptions{
			ClientOptions: clientOpt.ClientOptions,
			TenantID:      tenantId,
		}
		cred, err := azidentity.NewDefaultAzureCredential(opt)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to new Default credential: %v", err)
		}
		return cred, clientOpt, nil
	}
}

func (f FlagSet) BuildCommonConfig() (config.CommonConfig, error) {
	cred, clientOpt, err := buildAzureSDKCredAndClientOpt(f)
	if err != nil {
		return config.CommonConfig{}, err
	}

	cfg := config.CommonConfig{
		SubscriptionId:       f.flagSubscriptionId,
		AzureSDKCredential:   cred,
		AzureSDKClientOption: *clientOpt,
		OutputDir:            f.flagOutputDir,
		ProviderVersion:      f.flagProviderVersion,
		ProviderName:         f.flagProviderName,
		DevProvider:          f.flagDevProvider,
		ContinueOnError:      f.flagContinue,
		BackendType:          f.flagBackendType,
		BackendConfig:        f.flagBackendConfig.Value(),
		FullConfig:           f.flagFullConfig,
		Parallelism:          f.flagParallelism,
		HCLOnly:              f.flagHCLOnly,
		ModulePath:           f.flagModulePath,
		TelemetryClient:      initTelemetryClient(f.flagSubscriptionId),
	}

	if f.flagAppend {
		cfg.OutputFileNames = config.OutputFileNames{
			TerraformFileName:   "terraform.aztfexport.tf",
			ProviderFileName:    "provider.aztfexport.tf",
			MainFileName:        "main.aztfexport.tf",
			ImportBlockFileName: "import.aztfexport.tf",
		}
	}

	if f.hflagTFClientPluginPath != "" {
		// #nosec G204
		tfc, err := tfclient.New(tfclient.Option{
			Cmd:    exec.Command(flagset.hflagTFClientPluginPath),
			Logger: hclog.NewNullLogger(),
		})
		if err != nil {
			return cfg, err
		}
		cfg.TFClient = tfc
	}

	return cfg, nil
}
