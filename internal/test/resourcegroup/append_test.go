package resourcegroup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/aztfy/internal/test"

	"github.com/Azure/aztfy/internal"
	"github.com/Azure/aztfy/internal/config"
	"github.com/hashicorp/terraform-exec/tfexec"
)

func TestAppendMode(t *testing.T) {
	t.Parallel()
	test.Precheck(t)
	d := test.NewData()
	tfexecPath := test.EnsureTF(t)
	provisionDir := t.TempDir()
	os.Chdir(provisionDir)
	if err := os.WriteFile("main.tf", []byte(fmt.Sprintf(`
provider "azurerm" {
  features {}
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
		t.Fatalf("created to create the TF config file: %v", err)
	}
	tf, err := tfexec.NewTerraform(provisionDir, tfexecPath)
	if err != nil {
		t.Fatalf("failed to new terraform: %v", err)
	}
	ctx := context.Background()
	if err := tf.Init(ctx); err != nil {
		t.Fatalf("terraform init failed: %v", err)
	}
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("terraform apply failed: %v", err)
	}
	defer func() {
		if err := tf.Destroy(ctx); err != nil {
			t.Logf("terraform destroy failed: %v", err)
		}
	}()

	// Import the first resource group
	aztfyDir := t.TempDir()
	cfg := config.GroupConfig{
		CommonConfig: config.CommonConfig{
			SubscriptionId: os.Getenv("ARM_SUBSCRIPTION_ID"),
			OutputDir:      aztfyDir,
			BackendType:    "local",
		},
		ResourceNamePattern: "t1",
	}
	cfg.ResourceGroupName = d.RandomRgName() + "1"
	cfg.ResourceNamePattern = "round1_"
	if err := internal.BatchImport(cfg, false); err != nil {
		t.Fatalf("failed to run first batch import: %v", err)
	}
	// Import the second resource group mutably
	cfg.Append = true
	cfg.ResourceGroupName = d.RandomRgName() + "2"
	cfg.ResourceNamePattern = "round2_"
	if err := internal.BatchImport(cfg, false); err != nil {
		t.Fatalf("failed to run second batch import: %v", err)
	}
	// Import the third resource group mutably
	cfg.Append = true
	cfg.ResourceGroupName = d.RandomRgName() + "3"
	cfg.ResourceNamePattern = "round3_"
	if err := internal.BatchImport(cfg, false); err != nil {
		t.Fatalf("failed to run second batch import: %v", err)
	}

	// Verify
	tf2, err := tfexec.NewTerraform(aztfyDir, tfexecPath)
	if err != nil {
		t.Fatalf("failed to new terraform: %v", err)
	}
	diff, err := tf2.Plan(ctx)
	if err != nil {
		t.Fatalf("terraform plan in the generated workspace failed: %v", err)
	}
	if diff {
		t.Fatalf("terraform plan shows diff")
	}
	state, err := tf2.ShowStateFile(ctx, filepath.Join(aztfyDir, "terraform.tfstate"))
	if err != nil {
		t.Fatalf("terraform state show in the generated workspace failed: %v", err)
	}
	if n := len(state.Values.RootModule.Resources); n != 3 {
		t.Fatalf("expected terrafied resource: %d, got=%d", 3, n)
	}
}
