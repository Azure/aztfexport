package query

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	internalconfig "github.com/Azure/aztfexport/internal/config"
	"github.com/Azure/aztfexport/pkg/config"

	"github.com/Azure/aztfexport/internal"
	"github.com/Azure/aztfexport/internal/test"
	"github.com/Azure/aztfexport/internal/utils"
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

	if err := os.WriteFile(filepath.Join(provisionDir, "main.tf"), fmt.Appendf([]byte{}, `
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
`, d.RandomRgName()), 0644); err != nil {
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

	cred, clientOpt := test.BuildCredAndClientOpt(t)

	// Import in non-recursive mode
	aztfexportDir := t.TempDir()
	cfg := internalconfig.NonInteractiveModeConfig{
		Config: config.Config{
			CommonConfig: config.CommonConfig{
				Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
				SubscriptionId:       os.Getenv("ARM_SUBSCRIPTION_ID"),
				AzureSDKCredential:   cred,
				AzureSDKClientOption: *clientOpt,
				OutputDir:            aztfexportDir,
				BackendType:          "local",
				DevProvider:          true,
				Parallelism:          1,
				ProviderName:         "azurerm",
			},
			ResourceNamePattern:  "res-",
			ARGPredicate:         fmt.Sprintf(`resourceGroup =~ "%s" and type =~ "microsoft.network/virtualnetworks"`, d.RandomRgName()),
			IncludeResourceGroup: true,
		},
		PlainUI: true,
	}
	t.Log("Importing in non-recursive mode")
	if err := utils.RemoveEverythingUnder(cfg.OutputDir); err != nil {
		t.Fatalf("failed to clean up the output directory: %v", err)
	}
	if err := internal.BatchImport(ctx, cfg); err != nil {
		t.Fatalf("failed to run batch import non-recursively: %v", err)
	}
	test.Verify(t, ctx, aztfexportDir, tfexecPath, 2)

	// Import in recursive mode
	t.Log("Importing in recursive mode")
	cfg.RecursiveQuery = true
	if err := utils.RemoveEverythingUnder(cfg.OutputDir); err != nil {
		t.Fatalf("failed to clean up the output directory: %v", err)
	}
	if err := internal.BatchImport(ctx, cfg); err != nil {
		t.Fatalf("failed to run batch import recursively: %v", err)
	}
	test.Verify(t, ctx, aztfexportDir, tfexecPath, 3)

	// Import in recusrive mode, but exclude the vnet and subet
	t.Log("Importing in recursive mode, but exclude the vnet and subnet")
	cfg.ExcludeAzureResources = []string{"subnets/internal$"}
	cfg.ExcludeTerraformResources = []string{"azurerm_virtual_network"}
	if err := utils.RemoveEverythingUnder(cfg.OutputDir); err != nil {
		t.Fatalf("failed to clean up the output directory: %v", err)
	}
	if err := internal.BatchImport(ctx, cfg); err != nil {
		t.Fatalf("failed to run batch import recursively: %v", err)
	}
	test.Verify(t, ctx, aztfexportDir, tfexecPath, 1)
}
