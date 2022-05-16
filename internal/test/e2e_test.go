package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-version"
	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/src"

	"github.com/Azure/aztfy/internal"
	"github.com/Azure/aztfy/internal/config"
	"github.com/hashicorp/terraform-exec/tfexec"
)

const TestToggleEnvVar = "AZTFY_E2E"

func precheck(t *testing.T) {
	variables := []string{
		TestToggleEnvVar,
		"ARM_CLIENT_ID",
		"ARM_CLIENT_SECRET",
		"ARM_SUBSCRIPTION_ID",
		"ARM_TENANT_ID",
	}
	for _, variable := range variables {
		value := os.Getenv(variable)
		if value == "" {
			t.Skipf("`%s` must be set for e2e tests!", variable)
		}
	}
}

func ensureTF(t *testing.T) string {
	i := install.NewInstaller()
	execPath, err := i.Ensure(context.Background(), []src.Source{
		&fs.Version{
			Product:     product.Terraform,
			Constraints: version.MustConstraints(version.NewConstraint(">=0.12")),
		},
	})
	if err != nil {
		t.Fatalf("failed to find a Terraform executable: %v", err)
	}
	return execPath
}

func runCase(t *testing.T, d Data, c Case) {
	tfexecPath := ensureTF(t)
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
	cfg := config.Config{
		SubscriptionId:    os.Getenv("ARM_SUBSCRIPTION_ID"),
		ResourceGroupName: d.RandomRgName(),
		OutputDir:         aztfyDir,
		ResourceMapping:   c.ResourceMapping(d),
		BackendType:       "local",
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
	if n, expect := len(state.Values.RootModule.Resources), len(c.ResourceMapping(d)); n != expect {
		t.Fatalf("expected terrified resource: %d, got=%d", expect, n)
	}
}

func TestComputeVMDisk(t *testing.T) {
	t.Parallel()
	precheck(t)
	c, d := CaseComputeVMDisk{}, NewData()
	runCase(t, d, c)
}
