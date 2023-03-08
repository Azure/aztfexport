package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Azure/aztfexport/internal/meta"
	"github.com/Azure/aztfexport/internal/utils"
	"github.com/urfave/cli/v2"
)

func commandBeforeFunc(fset *FlagSet) func(ctx *cli.Context) error {
	return func(_ *cli.Context) error {
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

		if flagLogLevel != "" {
			if _, err := logLevel(flagLogLevel); err != nil {
				return err
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
				if err := utils.RemoveEverythingUnder(fset.flagOutputDir, meta.ResourceMappingFileName); err != nil {
					return fmt.Errorf("failed to clean up output directory %q: %v", fset.flagOutputDir, err)
				}
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

* Press "Y" to overwrite the existing directory with new files
* Press "N" to append new files and add to the existing state instead
* Press other keys to quit

> `)
				var ans string
				// #nosec G104
				fmt.Scanf("%s", &ans)
				switch strings.ToLower(ans) {
				case "y":
					if err := utils.RemoveEverythingUnder(fset.flagOutputDir, meta.ResourceMappingFileName); err != nil {
						return err
					}
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
