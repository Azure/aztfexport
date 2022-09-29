package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Azure/aztfy/internal"
	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/test"
	"github.com/Azure/aztfy/internal/utils"
	"github.com/hashicorp/terraform-exec/tfexec"
)

func TestQueryMode(t *testing.T) {
	t.Parallel()
	test.Precheck(t)
	d := test.NewData()
	tfexecPath := test.EnsureTF(t)

	provisionDir := t.TempDir()
	if test.Keep() {
		provisionDir, _ = os.MkdirTemp("", "")
		t.Log(provisionDir)
	}

	os.Chdir(provisionDir)
	if err := utils.WriteFileSync("main.tf", []byte(fmt.Sprintf(`
provider "azurerm" {
  features {
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
  }
}
resource "azurerm_resource_group" "test" {
  name     = "%[1]s"
  location = "WestEurope"
}
resource "azurerm_virtual_network" "test" {
  address_space       = ["10.0.0.0/16"]
  location            = "westeurope"
  name     			  = "%[1]s"
  resource_group_name = azurerm_resource_group.test.name
}
resource "azurerm_subnet" "test" {
  address_prefixes     = ["10.0.2.0/24"]
  name                 = "internal"
  resource_group_name  = azurerm_virtual_network.test.resource_group_name
  virtual_network_name = azurerm_virtual_network.test.name
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

	const delay = time.Minute
	t.Logf("Sleep for %v to wait for the just created resources be recorded in ARG\n", delay)
	time.Sleep(delay)

	// Import in non-recursive mode
	aztfyDir := t.TempDir()
	cfg := config.Config{
		CommonConfig: config.CommonConfig{
			SubscriptionId: os.Getenv("ARM_SUBSCRIPTION_ID"),
			OutputDir:      aztfyDir,
			BackendType:    "local",
			DevProvider:    true,
			PlainUI:        true,
			Overwrite:      true,
		},
		ResourceNamePattern: "res-",
		ARGPredicate:        fmt.Sprintf(`resourceGroup =~ "%s" and type =~ "microsoft.network/virtualnetworks"`, d.RandomRgName()),
	}
	t.Log("Importing in non-recursive mode")
	if err := internal.BatchImport(cfg, false); err != nil {
		t.Fatalf("failed to run batch import non-recursively: %v", err)
	}
	test.Verify(t, ctx, aztfyDir, tfexecPath, 1)

	// Import in recursive mode
	t.Log("Importing in recursive mode")
	// aztfyDir = t.TempDir()
	// cfg.CommonConfig.OutputDir = aztfyDir
	cfg.RecursiveQuery = true
	if err := internal.BatchImport(cfg, false); err != nil {
		t.Fatalf("failed to run batch import recursively: %v", err)
	}
	test.Verify(t, ctx, aztfyDir, tfexecPath, 2)
}
