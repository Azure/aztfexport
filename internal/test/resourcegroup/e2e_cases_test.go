package resourcegroup

import (
	"context"
	"github.com/Azure/aztfy/internal"
	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/test"
	"github.com/Azure/aztfy/internal/test/cases"
	"github.com/hashicorp/terraform-exec/tfexec"
	"os"
	"path/filepath"
	"testing"
)

func runCase(t *testing.T, d test.Data, c cases.Case) {
	tfexecPath := test.EnsureTF(t)
	provisionDir := t.TempDir()
	os.Chdir(provisionDir)
	if err := os.WriteFile("main.tf", []byte(c.Tpl(d)), 0644); err != nil {
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

	aztfyDir := t.TempDir()
	resourceMapping, err := c.ResourceMapping(d)
	if err != nil {
		t.Fatalf("failed to get resource mapping: %v", err)
	}
	cfg := config.RgConfig{
		CommonConfig: config.CommonConfig{
			SubscriptionId: os.Getenv("ARM_SUBSCRIPTION_ID"),
			OutputDir:      aztfyDir,
			BackendType:    "local",
		},
		ResourceGroupName: d.RandomRgName(),
		ResourceMapping:   resourceMapping,
	}
	if err := internal.BatchImport(cfg, false); err != nil {
		t.Fatalf("failed to run batch import: %v", err)
	}
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
	if n, expect := len(state.Values.RootModule.Resources), len(resourceMapping); n != expect {
		t.Fatalf("expected terrafied resource: %d, got=%d", expect, n)
	}
}

func TestComputeVMDisk(t *testing.T) {
	t.Parallel()
	test.Precheck(t)
	c, d := cases.CaseComputeVMDisk{}, test.NewData()
	runCase(t, d, c)
}

func TestSignalRService(t *testing.T) {
	t.Parallel()
	test.Precheck(t)
	c, d := cases.CaseSignalRService{}, test.NewData()
	runCase(t, d, c)
}

func TestApplicationInsightWebTest(t *testing.T) {
	t.Parallel()
	test.Precheck(t)
	c, d := cases.CaseApplicationInsightWebTest{}, test.NewData()
	runCase(t, d, c)
}

func TestKeyVaultNestedItems(t *testing.T) {
	t.Parallel()
	test.Precheck(t)
	c, d := cases.CaseKeyVaultNestedItems{}, test.NewData()
	runCase(t, d, c)
}

func TestFunctionAppSlot(t *testing.T) {
	t.Parallel()
	test.Precheck(t)
	c, d := cases.CaseFunctionAppSlot{}, test.NewData()
	runCase(t, d, c)
}

func TestStorageFileShare(t *testing.T) {
	t.Parallel()
	test.Precheck(t)
	c, d := cases.CaseStorageFileShare{}, test.NewData()
	runCase(t, d, c)
}
