package resourcegroup

import (
	"context"
	"fmt"
	internalconfig "github.com/Azure/aztfy/internal/config"
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/aztfy/pkg/config"

	"github.com/Azure/aztfy/internal/test"
	"github.com/Azure/aztfy/internal/utils"

	"github.com/Azure/aztfy/internal"
	"github.com/hashicorp/terraform-exec/tfexec"
)

func TestAppendMode(t *testing.T) {
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
	cfg := internalconfig.NonInteractiveModeConfig{
		Config: config.Config{
			CommonConfig: config.CommonConfig{
				SubscriptionId: os.Getenv("ARM_SUBSCRIPTION_ID"),
				OutputDir:      aztfyDir,
				BackendType:    "local",
				DevProvider:    true,
				Parallelism:    1,
			},
			ResourceNamePattern: "t1",
		},
		PlainUI: true,
	}
	cfg.ResourceGroupName = d.RandomRgName() + "1"
	cfg.ResourceNamePattern = "round1_"
	t.Log("Batch importing the 1st rg")
	if err := utils.RemoveEverythingUnder(cfg.OutputDir); err != nil {
		t.Fatalf("failed to clean up the output directory: %v", err)
	}
	if err := internal.BatchImport(cfg); err != nil {
		t.Fatalf("failed to run first batch import: %v", err)
	}
	// Import the second resource group mutably
	cfg.Append = true
	cfg.ResourceGroupName = d.RandomRgName() + "2"
	cfg.ResourceNamePattern = "round2_"
	t.Log("Batch importing the 2nd rg")
	if err := internal.BatchImport(cfg); err != nil {
		t.Fatalf("failed to run second batch import: %v", err)
	}
	// Import the third resource group mutably
	cfg.Append = true
	cfg.ResourceGroupName = d.RandomRgName() + "3"
	cfg.ResourceNamePattern = "round3_"
	t.Log("Batch importing the 3rd rg")
	if err := internal.BatchImport(cfg); err != nil {
		t.Fatalf("failed to run second batch import: %v", err)
	}

	// Verify
	test.Verify(t, ctx, aztfyDir, tfexecPath, 3)
}
