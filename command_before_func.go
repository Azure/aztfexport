package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Azure/aztfexport/internal/utils"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/urfave/cli/v2"
)

func commandBeforeFunc(fset *FlagSet, mode Mode) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		// Common flags check
		if fset.flagAppend {
			if fset.flagOverwrite {
				return fmt.Errorf("`--append` conflicts with `--overwrite`")
			}
		}
		if !fset.flagNonInteractive {
			if fset.flagContinue {
				return fmt.Errorf("`--continue` must be used together with `--non-interactive`")
			}
			if fset.flagGenerateMappingFile {
				return fmt.Errorf("`--generate-mapping-file` must be used together with `--non-interactive`")
			}
		}
		if fset.flagHCLOnly {
			if fset.flagAppend {
				return fmt.Errorf("`--append` conflicts with `--hcl-only`")
			}
			if fset.flagModulePath != "" {
				return fmt.Errorf("`--module-path` conflicts with `--hcl-only`")
			}
		}
		if fset.flagModulePath != "" {
			if !fset.flagAppend {
				return fmt.Errorf("`--module-path` must be used together with `--append`")
			}
		}
		if fset.flagDevProvider {
			if fset.flagProviderVersion != "" {
				return fmt.Errorf("`--dev-provider` conflicts with `--provider-version`")
			}
		}
		if fset.hflagTFClientPluginPath != "" {
			if !fset.flagHCLOnly {
				return fmt.Errorf("`--tfclient-plugin-path` must be used together with `--hcl-only`")
			}
		}

		if err := conflictArgs([]argDesc{
			{
				name:  "--client-id",
				isSet: fset.flagClientId != "",
			},
			{
				name:  "--client-id-file-path",
				isSet: fset.flagClientIdFilePath != "",
			},
		}); err != nil {
			return err
		}

		if err := conflictArgs([]argDesc{
			{
				name:  "--client-certificate",
				isSet: fset.flagClientCertificate != "",
			},
			{
				name:  "--client-certificate-path",
				isSet: fset.flagClientCertificatePath != "",
			},
		}); err != nil {
			return err
		}

		if err := conflictArgs([]argDesc{
			{
				name:  "--client-secret",
				isSet: fset.flagClientSecret != "",
			},
			{
				name:  "--client-secret-file-path",
				isSet: fset.flagClientSecretFilePath != "",
			},
		}); err != nil {
			return err
		}

		if err := conflictArgs([]argDesc{
			{
				name:  "--oidc-token",
				isSet: fset.flagOIDCToken != "",
			},
			{
				name:  "--oidc-token-file-path",
				isSet: fset.flagOIDCTokenFilePath != "",
			},
		}); err != nil {
			return err
		}

		// Mode specific flags check
		switch mode {
		case ModeResource:
			if ctx.Args().Len() > 1 || (ctx.Args().Len() == 1 && strings.HasPrefix(ctx.Args().First(), "@")) {
				if fset.flagResType != "" {
					return fmt.Errorf("`--type` can't be specified for multi-resource mode")
				}
				if fset.flagResName != "" {
					return fmt.Errorf("`--name` can't be specified for multi-resource mode")
				}
			}
		case ModeQuery:
			if fset.flagARGAuthorizationScopeFilter != "" {
				if !slices.Contains(armresourcegraph.PossibleAuthorizationScopeFilterValues(), armresourcegraph.AuthorizationScopeFilter(fset.flagARGAuthorizationScopeFilter)) {
					return fmt.Errorf("invalid value of `--arg-authorization-scope-filter`")
				}
			}
		}

		// Initialize output directory
		if _, err := os.Stat(fset.flagOutputDir); os.IsNotExist(err) {
			if err := os.MkdirAll(fset.flagOutputDir, 0750); err != nil {
				return fmt.Errorf("creating output directory %q: %v", fset.flagOutputDir, err)
			}
		}
		empty, err := utils.DirIsEmpty(fset.flagOutputDir)
		if err != nil {
			return fmt.Errorf("failed to check emptiness of output directory %q: %v", fset.flagOutputDir, err)
		}

		var tfblock *utils.TerraformBlockDetail
		if !empty {
			switch {
			case fset.flagOverwrite:
			case fset.flagAppend:
				tfblock, err = utils.InspecTerraformBlock(fset.flagOutputDir)
				if err != nil {
					return fmt.Errorf("determine the backend type from the existing files: %v", err)
				}
			default:
				if fset.flagNonInteractive {
					return fmt.Errorf("the output directory %q is not empty", fset.flagOutputDir)
				}

				// Interactive mode
				fmt.Printf(`
The output directory is not empty. Please choose one of actions below:

* Press "Y" to proceed that will likely pollute the existing files and cause errors
* Press "N" to append new files and add to the existing state instead
* Press other keys to quit

> `)
				var ans string
				// #nosec G104
				fmt.Scanf("%s", &ans)
				switch strings.ToLower(ans) {
				case "y":
				case "n":
					if fset.flagHCLOnly {
						return fmt.Errorf("`--hcl-only` can only run within an empty directory. Use `-o` to specify an empty directory.")
					}
					fset.flagAppend = true
					tfblock, err = utils.InspecTerraformBlock(fset.flagOutputDir)
					if err != nil {
						return fmt.Errorf("determine the backend type from the existing files: %v", err)
					}
				default:
					return fmt.Errorf("the output directory %q is not empty", fset.flagOutputDir)
				}
			}
		}

		// Deterimine the real backend type to use
		var existingBackendType string
		if tfblock != nil {
			existingBackendType = "local"
			if tfblock.BackendType != "" {
				existingBackendType = tfblock.BackendType
			}
		}
		switch {
		case fset.flagBackendType != "" && existingBackendType != "":
			if fset.flagBackendType != existingBackendType {
				return fmt.Errorf("the backend type defined in existing files (%s) are not the same as is specified in the CLI (%s)", existingBackendType, fset.flagBackendType)
			}
		case fset.flagBackendType == "" && existingBackendType == "":
			fset.flagBackendType = "local"
		case fset.flagBackendType == "" && existingBackendType != "":
			fset.flagBackendType = existingBackendType
		case fset.flagBackendType != "" && existingBackendType == "":
			// do nothing
		}

		// Check backend related flags
		if len(fset.flagBackendConfig.Value()) != 0 {
			if existingBackendType != "" {
				return fmt.Errorf("`--backend-config` should not be specified when appending to a workspace that has terraform block already defined")
			}
			if fset.flagBackendType == "local" {
				return fmt.Errorf("`--backend-config` only works for non-local backend")
			}
		}
		if fset.flagBackendType != "local" {
			if fset.flagHCLOnly {
				return fmt.Errorf("`--hcl-only` only works for local backend")
			}
		}

		// Determine any existing provider version constraint if not using a dev provider and the provider version not specified.
		if !fset.flagDevProvider && fset.flagProviderVersion == "" {
			module, err := tfconfig.LoadModule(fset.flagOutputDir)
			if err != nil {
				return fmt.Errorf("loading terraform config: %v", err)
			}
			if azurecfg, ok := module.RequiredProviders[fset.flagProviderName]; ok {
				fset.flagProviderVersion = strings.Join(azurecfg.VersionConstraints, " ")
			}
		}

		// Identify the subscription id, which comes from one of following (starts from the highest priority):
		// - Command line option
		// - Env variable: AZTFEXPORT_SUBSCRIPTION_ID
		// - Env variable: ARM_SUBSCRIPTION_ID
		// - Output of azure cli, the current active subscription
		if fset.flagSubscriptionId == "" {
			var err error
			fset.flagSubscriptionId, err = subscriptionIdFromCLI()
			if err != nil {
				return fmt.Errorf("retrieving subscription id from CLI: %v", err)
			}
		}
		return nil
	}
}

type argDesc struct {
	name  string
	isSet bool
}

func conflictArgs(argDescs []argDesc) error {
	var conflictArgs []string
	for _, desc := range argDescs {
		if desc.isSet {
			conflictArgs = append(conflictArgs, fmt.Sprintf("%q", desc.name))
		}
	}
	if len(conflictArgs) > 1 {
		return fmt.Errorf("only one of the followings can be specified: %s", strings.Join(conflictArgs, ", "))
	}
	return nil
}
