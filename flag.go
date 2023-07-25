package main

import (
	"fmt"
	"strings"

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
	//
	// query:
	// flagPattern
	// flagRecursive
	flagPattern   string
	flagRecursive bool
	flagResName   string
	flagResType   string
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
	case ModeQuery:
		if flag.flagPattern != "" {
			args = append(args, "--name-pattern="+flag.flagPattern)
		}
		if flag.flagRecursive {
			args = append(args, "--recursive=true")
		}
	}
	return "aztfexport " + strings.Join(args, " ")
}
