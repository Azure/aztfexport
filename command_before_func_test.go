package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestCommondBeforeFunc(t *testing.T) {
	dirGenEmpty := func(t *testing.T) string {
		return t.TempDir()
	}
	dirGenWithTFBlock := func(content string) func(t *testing.T) string {
		return func(t *testing.T) string {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "terraform.tf"), []byte(content), 0640); err != nil {
				t.Fatal(err)
			}
			return dir
		}
	}

	cases := []struct {
		name      string
		fset      FlagSet
		dirGen    func(t *testing.T) string
		err       string
		postCheck func(t *testing.T, flagset FlagSet)
	}{
		{
			name: "--append conflicts with --overwrite",
			fset: FlagSet{
				flagAppend:    true,
				flagOverwrite: true,
			},
			err: "`--append` conflicts with `--overwrite`",
		},
		{
			name: "only a --append works",
			fset: FlagSet{
				flagAppend:         true,
				flagSubscriptionId: "123",
			},
		},
		{
			name: "only a --overwrite works",
			fset: FlagSet{
				flagOverwrite: true,
			},
		},
		{
			name: "--continue shouldn't be used in interactive mode since interactive mode can toggle off the failed resources",
			fset: FlagSet{
				flagContinue: true,
			},
			err: "`--continue` must be used together with `--non-interactive`",
		},
		{
			name: "--continue with --non-interactive works",
			fset: FlagSet{
				flagContinue:       true,
				flagNonInteractive: true,
			},
		},
		{
			name: "--generate-mapping-file shouldn't be used in interactive mode since interactive mode has a special code to do it",
			fset: FlagSet{
				flagGenerateMappingFile: true,
			},
			err: "`--generate-mapping-file` must be used together with `--non-interactive`",
		},
		{
			name: "--generate-mapping-file with --non-interactive works",
			fset: FlagSet{
				flagGenerateMappingFile: true,
				flagNonInteractive:      true,
			},
		},
		{
			name: "--hcl-only shouldn't be used with --append since it doesn't make sense to generate config/state to an existing workspace for hcl only",
			fset: FlagSet{
				flagHCLOnly: true,
				flagAppend:  true,
			},
			err: "`--append` conflicts with `--hcl-only`",
		},
		{
			name: "--hcl-only works alone",
			fset: FlagSet{
				flagHCLOnly: true,
			},
		},
		{
			name: "--module-path shouldn't be used with --hcl-only since --module-path will be used together with --append",
			fset: FlagSet{
				flagHCLOnly:    true,
				flagModulePath: "foo",
			},
			err: "`--module-path` conflicts with `--hcl-only`",
		},
		{
			name: "--module-path should be used together with --append",
			fset: FlagSet{
				flagModulePath: "foo",
			},
			err: "`--module-path` must be used together with `--append`",
		},
		{
			name: "--module-path with --append works",
			fset: FlagSet{
				flagModulePath: "foo",
				flagAppend:     true,
			},
		},
		{
			name: "--dev-provider conflicts with --provider-version",
			fset: FlagSet{
				flagDevProvider:     true,
				flagProviderVersion: "= 1.2.3",
			},
			err: "`--dev-provider` conflicts with `--provider-version`",
		},
		{
			name: "non empty dir but overwrite",
			fset: FlagSet{
				flagOverwrite: true,
			},
			dirGen: dirGenWithTFBlock("foo {}"),
		},
		{
			name: "default backend type is local",
			fset: FlagSet{},
			postCheck: func(t *testing.T, flagset FlagSet) {
				require.Equal(t, "local", flagset.flagBackendType)
			},
		},
		{
			name: "append to a dir with no terraform config ends up backend type local",
			fset: FlagSet{
				flagAppend: true,
			},
			dirGen: dirGenWithTFBlock("foo {}"),
			postCheck: func(t *testing.T, flagset FlagSet) {
				require.Equal(t, "local", flagset.flagBackendType)
			},
		},
		{
			name: "append to a dir with empty terraform config ends up backend type local, which conflicts with the specified backend type",
			fset: FlagSet{
				flagBackendType: "azurerm",
				flagAppend:      true,
			},
			dirGen: dirGenWithTFBlock(`terraform {}`),
			err:    "the backend type defined in existing files (local) are not the same as is specified in the CLI (azurerm)",
		},
		{
			name: "append to a dir with terraform config of backend type set to local, which conflicts with the specified backend type",
			fset: FlagSet{
				flagBackendType: "azurerm",
				flagAppend:      true,
			},
			dirGen: dirGenWithTFBlock(`terraform {
	backend local {}
}`),
			err: "the backend type defined in existing files (local) are not the same as is specified in the CLI (azurerm)",
		},
		{
			name: "append to a dir with terraform config of backend type set to azurerm, which aligns with the specified backend type",
			fset: FlagSet{
				flagBackendType: "azurerm",
				flagAppend:      true,
			},
			dirGen: dirGenWithTFBlock(`terraform {
	backend azurerm {}
}`),
		},
		{
			name: "append to a dir with terraform config of backend type set to foo, which conflicts with the specified backend type",
			fset: FlagSet{
				flagBackendType: "azurerm",
				flagAppend:      true,
			},
			dirGen: dirGenWithTFBlock(`terraform {
	backend foo {}
}`),
			err: "the backend type defined in existing files (foo) are not the same as is specified in the CLI (azurerm)",
		},
		{
			name: "--backend-config shouldn't be used with local backend",
			fset: FlagSet{
				flagBackendConfig: *cli.NewStringSlice("foo=bar"),
			},
			err: "`--backend-config` only works for non-local backend",
		},
		{
			name: "--backend-config shouldn't be used when appending to a workspace with backend config defined",
			fset: FlagSet{
				flagAppend:        true,
				flagBackendConfig: *cli.NewStringSlice("foo=bar"),
			},
			dirGen: dirGenWithTFBlock(`terraform {}`),
			err:    "`--backend-config` should not be specified when appending to a workspace that has terraform block already defined",
		},
		{
			name: "--hcl-only can't work for remote backend",
			fset: FlagSet{
				flagBackendType: "azurerm",
				flagHCLOnly:     true,
			},
			err: "`--hcl-only` only works for local backend",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.dirGen == nil {
				tt.dirGen = dirGenEmpty
			}
			tt.fset.flagOutputDir = tt.dirGen(t)

			// This is to avoid reading the subscription id from az cli, which is not setup in CI.
			if tt.fset.flagSubscriptionId == "" {
				tt.fset.flagSubscriptionId = "test"
			}

			err := commandBeforeFunc(&tt.fset)(nil)
			if tt.err == "" {
				require.NoError(t, err)
				if tt.postCheck != nil {
					tt.postCheck(t, tt.fset)
				}
				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}
