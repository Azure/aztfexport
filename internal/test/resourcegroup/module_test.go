package resourcegroup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	internalconfig "github.com/Azure/aztfy/internal/config"

	"github.com/Azure/aztfy/pkg/config"

	"github.com/Azure/aztfy/internal/test"
	"github.com/stretchr/testify/require"

	"github.com/Azure/aztfy/internal"
	"github.com/hashicorp/terraform-exec/tfexec"
)

func TestAppendToModule(t *testing.T) {
	t.Parallel()
	test.Precheck(t)
	d := test.NewData()
	tfexecPath := test.EnsureTF(t)

	provisionDir := t.TempDir()
	if test.Keep() {
		provisionDir, _ = os.MkdirTemp("", "")
		t.Log(provisionDir)
	}

	if err := os.WriteFile(filepath.Join(provisionDir, "main.tf"), []byte(fmt.Sprintf(`
provider "azurerm" {
  features {
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
  }
}
resource "azurerm_resource_group" "test1" {
  name     = "%[1]s1"
  location = "WestEurope"
}
resource "azurerm_resource_group" "test2" {
  name     = "%[1]s2"
  location = "WestEurope"
}
resource "azurerm_resource_group" "test3" {
  name     = "%[1]s3"
  location = "WestEurope"
}
`, d.RandomRgName())), 0644); err != nil {
		t.Fatalf("failed to create the TF config file: %v", err)
	}
	tf, err := tfexec.NewTerraform(provisionDir, tfexecPath)
	if err != nil {
		t.Fatalf("failed to new terraform: %v", err)
	}
	ctx := context.Background()
	t.Log("Running: terraform init")
	if err := tf.Init(ctx); err != nil {
		t.Fatalf("terraform init failed: %v", err)
	}
	t.Log("Running: terraform apply")
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("terraform apply failed: %v", err)
	}

	if !test.Keep() {
		defer func() {
			t.Log("Running: terraform destroy")
			if err := tf.Destroy(ctx); err != nil {
				t.Logf("terraform destroy failed: %v", err)
			}
		}()
	}

	// Import the first resource group
	aztfyDir := t.TempDir()

	tf, err = tfexec.NewTerraform(aztfyDir, tfexecPath)
	if err != nil {
		t.Fatalf("failed to new terraform: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(aztfyDir, "modules", "submodules"), 0755); err != nil {
		t.Fatalf("failed to create the directory `modules/submodules`: %v", err)
	}
	if err := os.WriteFile(filepath.Join(aztfyDir, "main.tf"), []byte(`
module "my-module" {
  source = "./modules"
}
`), 0644); err != nil {
		t.Fatalf("failed to create the TF config file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(aztfyDir, "modules", "main.tf"), []byte(`
module "sub-module" {
  source = "./submodules"
}
`), 0644); err != nil {
		t.Fatalf("failed to create the TF config file: %v", err)
	}
	t.Log("Running: terraform init")
	if err := tf.Init(ctx); err != nil {
		t.Fatalf("terraform init failed: %v", err)
	}

	cred, clientOpt := test.BuildCredAndClientOpt(t)

	cfg := internalconfig.NonInteractiveModeConfig{
		Config: config.Config{
			CommonConfig: config.CommonConfig{
				SubscriptionId:       os.Getenv("ARM_SUBSCRIPTION_ID"),
				AzureSDKCredential:   cred,
				AzureSDKClientOption: *clientOpt,
				OutputDir:            aztfyDir,
				BackendType:          "local",
				Parallelism:          1,
				ModulePath:           "", // Import to the root module
			},
		},
		PlainUI: true,
	}
	cfg.ResourceGroupName = d.RandomRgName() + "1"
	cfg.ResourceNamePattern = "round1_"
	t.Log("Batch importing the 1st rg")
	if err := internal.BatchImport(ctx, cfg); err != nil {
		t.Fatalf("failed to run first batch import: %v", err)
	}
	// Import the second resource group mutably
	cfg.ResourceGroupName = d.RandomRgName() + "2"
	cfg.ResourceNamePattern = "round2_"
	cfg.CommonConfig.ModulePath = "my-module"
	t.Log("Batch importing the 2nd rg")
	if err := internal.BatchImport(ctx, cfg); err != nil {
		t.Fatalf("failed to run second batch import: %v", err)
	}
	// Import the third resource group mutably
	cfg.ResourceGroupName = d.RandomRgName() + "3"
	cfg.ResourceNamePattern = "round3_"
	cfg.CommonConfig.ModulePath = "my-module.sub-module"
	t.Log("Batch importing the 3rd rg")
	if err := internal.BatchImport(ctx, cfg); err != nil {
		t.Fatalf("failed to run second batch import: %v", err)
	}

	// Verify
	t.Log("Running: terraform plan")
	planFile := filepath.Join(t.TempDir(), "plan")
	diff, err := tf.Plan(ctx, tfexec.Out(planFile))
	if err != nil {
		t.Fatalf("terraform plan in the generated workspace failed: %v", err)
	}
	if diff {
		t.Fatalf("terraform plan has diff")
	}
	t.Log("Running: terraform show")
	state, err := tf.ShowStateFile(ctx, filepath.Join(aztfyDir, "terraform.tfstate"))
	if err != nil {
		t.Fatalf("terraform state show in the generated workspace failed: %v", err)
	}
	require.Equal(t, 1, len(state.Values.RootModule.Resources))
	require.Equal(t, 1, len(state.Values.RootModule.ChildModules[0].Resources))
	require.Equal(t, 1, len(state.Values.RootModule.ChildModules[0].ChildModules[0].Resources))
}
