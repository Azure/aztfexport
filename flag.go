package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/Azure/aztfexport/internal/cfgfile"
	"github.com/Azure/aztfexport/internal/log"
	"github.com/Azure/aztfexport/pkg/config"
	"github.com/Azure/aztfexport/pkg/telemetry"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	azlog "github.com/Azure/azure-sdk-for-go/sdk/azcore/log"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/gofrs/uuid"
	"github.com/urfave/cli/v2"
)

var flagset FlagSet

type FlagSet struct {
	// common flags
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
	flagMaskSensitive       bool
	flagParallelism         int
	flagContinue            bool
	flagNonInteractive      bool
	flagPlainUI             bool
	flagGenerateMappingFile bool
	flagHCLOnly             bool
	flagModulePath          string
	flagGenerateImportBlock bool
	flagLogPath             string
	flagLogLevel            string

	// common flags (auth)
	flagEnv                       string
	flagTenantId                  string
	flagAuxiliaryTenantIds        cli.StringSlice
	flagClientId                  string
	flagClientIdFilePath          string
	flagClientCertificate         string
	flagClientCertificatePath     string
	flagClientCertificatePassword string
	flagClientSecret              string
	flagClientSecretFilePath      string
	flagOIDCRequestToken          string
	flagOIDCRequestURL            string
	flagOIDCTokenFilePath         string
	flagOIDCToken                 string
	flagUseManagedIdentityCred    bool
	flagUseAzureCLICred           bool
	flagUseOIDCCred               bool

	// common flags (hidden)
	hflagMockClient         bool
	hflagProfile            string
	hflagTFClientPluginPath string

	// Subcommand specific flags
	//
	// res:
	// flagResName (for single resource)
	// flagResType (for single resource)
	// flagPattern (for multi resources)
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
	// flagARGTable
	// flagARGAuthorizationScopeFilter
	flagPattern                     string
	flagRecursive                   bool
	flagResName                     string
	flagResType                     string
	flagIncludeRoleAssignment       bool
	flagIncludeResourceGroup        bool
	flagARGTable                    string
	flagARGAuthorizationScopeFilter string
}

type Mode string

const (
	ModeResource      Mode = "resource"
	ModeResourceGroup Mode = "resource-group"
	ModeQuery         Mode = "query"
	ModeMappingFile   Mode = "mapping-file"
)

// DescribeCLI construct a description of the CLI based on the flag set and the specified mode.
// The main reason is to record the usage of some "interesting" options in the telemetry.
// Note that only insensitive values are recorded (i.e. subscription id, resource id, etc are not recorded)
func (flag FlagSet) DescribeCLI(mode Mode) string {
	args := []string{string(mode)}

	// The following flags are skipped eiter not interesting, or might contain sensitive info:
	// - flagOutputDir
	// - flagDevProvider
	// - flagBackendConfig
	// - all hflags

	if flag.flagSubscriptionId != "" {
		args = append(args, "--subscription=*")
	}
	if flag.flagOverwrite {
		args = append(args, "--overwrite=true")
	}
	if flag.flagAppend {
		args = append(args, "--append=true")
	}
	if flag.flagProviderVersion != "" {
		args = append(args, fmt.Sprintf(`-provider-version=%s`, flag.flagProviderVersion))
	}
	if flag.flagProviderName != "" {
		args = append(args, fmt.Sprintf(`-provider-name=%s`, flag.flagProviderName))
	}
	if flag.flagBackendType != "" {
		args = append(args, "--backend-type="+flag.flagBackendType)
	}
	if flag.flagFullConfig {
		args = append(args, "--full-properties=true")
	}
	if flag.flagMaskSensitive {
		args = append(args, "--mask-sensitive=true")
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
	if !flag.flagGenerateImportBlock {
		args = append(args, "--generate-import-block=true")
	}

	if flag.flagEnv != "" {
		args = append(args, "--env="+flag.flagEnv)
	}
	if flag.flagTenantId != "" {
		args = append(args, "--tenant-id=*")
	}
	if v := flag.flagAuxiliaryTenantIds.Value(); len(v) != 0 {
		args = append(args, fmt.Sprintf("--tenant-id=[%d]", len(v)))
	}
	if flag.flagClientId != "" {
		args = append(args, "--client-id=*")
	}
	if flag.flagClientIdFilePath != "" {
		args = append(args, "--client-id-file-path="+flag.flagClientIdFilePath)
	}
	if flag.flagClientCertificate != "" {
		args = append(args, "--client-certificate=*")
	}
	if flag.flagClientCertificatePath != "" {
		args = append(args, "--client-certificate-path="+flag.flagClientCertificatePath)
	}
	if flag.flagClientCertificatePassword != "" {
		args = append(args, "--client-certificate-password=*")
	}
	if flag.flagClientSecret != "" {
		args = append(args, "--client-secret=***")
	}
	if flag.flagClientSecretFilePath != "" {
		args = append(args, "--client-secret-file-path="+flag.flagClientSecretFilePath)
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
	if flag.flagUseManagedIdentityCred {
		args = append(args, "--use-managed-identity-cred=true")
	}
	if flag.flagUseAzureCLICred {
		args = append(args, "--use-azure-cli-cred=true")
	}
	if flag.flagUseOIDCCred {
		args = append(args, "--use-oidc-cred=true")
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
		if flag.flagPattern != "" {
			args = append(args, "--name-pattern="+flag.flagPattern)
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
		if flag.flagARGTable != "" {
			args = append(args, "--arg-table="+flag.flagARGTable)
		}
		if flag.flagARGAuthorizationScopeFilter != "" {
			args = append(args, "--arg-authorization-scope-filter="+flag.flagARGAuthorizationScopeFilter)
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

func (f FlagSet) buildAuthConfig() (*config.AuthConfig, error) {
	clientId := f.flagClientId
	if path := f.flagClientIdFilePath; path != "" {
		// #nosec G304
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading Client ID from file %q: %v", path, err)
		}
		clientId = string(b)
	}

	clientSecret := f.flagClientSecret
	if path := f.flagClientSecretFilePath; path != "" {
		// #nosec G304
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading Client secret from file %q: %v", path, err)
		}
		clientSecret = string(b)
	}

	clientCertEncoded := f.flagClientCertificate
	if path := f.flagClientCertificatePath; path != "" {
		// #nosec G304
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading Client certificate from file %q: %v", path, err)
		}
		clientCertEncoded = base64.StdEncoding.EncodeToString(b)
	}

	oidcToken := f.flagOIDCToken
	if path := f.flagOIDCTokenFilePath; path != "" {
		// #nosec G304
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading OIDC token from file %q: %v", path, err)
		}
		oidcToken = strings.TrimSpace(string(b))
	}

	c := config.AuthConfig{
		Environment:               f.flagEnv,
		TenantID:                  f.flagTenantId,
		AuxiliaryTenantIDs:        f.flagAuxiliaryTenantIds.Value(),
		ClientID:                  clientId,
		ClientSecret:              clientSecret,
		ClientCertificateEncoded:  clientCertEncoded,
		ClientCertificatePassword: f.flagClientCertificatePassword,
		OIDCTokenRequestToken:     f.flagOIDCRequestToken,
		OIDCTokenRequestURL:       f.flagOIDCRequestURL,
		OIDCAssertionToken:        oidcToken,
		UseAzureCLI:               f.flagUseAzureCLICred,
		UseManagedIdentity:        f.flagUseManagedIdentityCred,
		UseOIDC:                   f.flagUseOIDCCred,
	}

	return &c, nil
}

// BuildCommonConfig builds the CommonConfig from the FlagSet, except the TFClient, which is built afterwards as it requires a logger.
func (f FlagSet) BuildCommonConfig() (config.CommonConfig, error) {
	// Logger is only enabled when the log path is specified.
	// This is because either interactive/non-interactive mode controls the terminal rendering,
	// logging to stdout/stderr will impact the rendering.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if path := f.flagLogPath; path != "" {
		level, err := logLevel(f.flagLogLevel)
		if err != nil {
			return config.CommonConfig{}, err
		}

		// #nosec G304
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return config.CommonConfig{}, fmt.Errorf("creating log file %s: %v", path, err)
		}

		logger = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level}))

		// Enable log for azure sdk
		os.Setenv("AZURE_SDK_GO_LOGGING", "all") // #nosec G104
		azlog.SetListener(func(cls azlog.Event, msg string) {
			logger.Log(context.Background(), log.LevelTrace, msg, "event", cls)
		})
	}

	authConfig, err := f.buildAuthConfig()
	if err != nil {
		return config.CommonConfig{}, err
	}

	var cloudCfg cloud.Configuration
	switch env := f.flagEnv; strings.ToLower(env) {
	case "public":
		cloudCfg = cloud.AzurePublic
	case "usgovernment":
		cloudCfg = cloud.AzureGovernment
	case "china":
		cloudCfg = cloud.AzureChina
	default:
		return config.CommonConfig{}, fmt.Errorf("unknown environment specified: %q", env)
	}

	clientOpt := arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloudCfg,
			Telemetry: policy.TelemetryOptions{
				ApplicationID: fmt.Sprintf("aztfexport(%s)", f.flagProviderName),
				Disabled:      false,
			},
			Logging: policy.LogOptions{
				IncludeBody: true,
			},
		},
		AuxiliaryTenants:      authConfig.AuxiliaryTenantIDs,
		DisableRPRegistration: true,
	}

	cred, err := NewDefaultAzureCredential(*logger, &DefaultAzureCredentialOptions{
		AuthConfig:               *authConfig,
		ClientOptions:            clientOpt.ClientOptions,
		DisableInstanceDiscovery: false,
		SendCertificateChain:     false,
	})
	if err != nil {
		return config.CommonConfig{}, fmt.Errorf("failed to new credential: %v", err)
	}

	cfg := config.CommonConfig{
		Logger:               logger,
		AuthConfig:           *authConfig,
		SubscriptionId:       f.flagSubscriptionId,
		AzureSDKCredential:   cred,
		AzureSDKClientOption: clientOpt,
		OutputDir:            f.flagOutputDir,
		ProviderVersion:      f.flagProviderVersion,
		ProviderName:         f.flagProviderName,
		DevProvider:          f.flagDevProvider,
		ContinueOnError:      f.flagContinue,
		BackendType:          f.flagBackendType,
		BackendConfig:        f.flagBackendConfig.Value(),
		FullConfig:           f.flagFullConfig,
		MaskSensitive:        f.flagMaskSensitive,
		Parallelism:          f.flagParallelism,
		HCLOnly:              f.flagHCLOnly,
		ModulePath:           f.flagModulePath,
		GenerateImportBlock:  f.flagGenerateImportBlock,
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

	return cfg, nil
}

func logLevel(level string) (slog.Level, error) {
	switch strings.ToUpper(level) {
	case "ERROR":
		return slog.LevelError, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "DEBUG":
		return slog.LevelDebug, nil
	case "TRACE":
		return log.LevelTrace, nil
	default:
		return slog.Level(0), fmt.Errorf("unknown log level: %s", level)
	}
}
