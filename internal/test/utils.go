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
	"github.com/hashicorp/terraform-exec/tfexec"
)

const TestToggleEnvVar = "AZTFY_E2E"

func Precheck(t *testing.T) {
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

func EnsureTF(t *testing.T) string {
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

func Verify(t *testing.T, ctx context.Context, aztfyDir, tfexecPath string, expectResCnt int) {
	tf, err := tfexec.NewTerraform(aztfyDir, tfexecPath)
	if err != nil {
		t.Fatalf("failed to new terraform: %v", err)
	}
	t.Log("Running: terraform plan")
	diff, err := tf.Plan(ctx)
	if err != nil {
		t.Fatalf("terraform plan in the generated workspace failed: %v", err)
	}
	if diff {
		t.Fatalf("terraform plan shows diff")
	}
	t.Log("Running: terraform show")
	state, err := tf.ShowStateFile(ctx, filepath.Join(aztfyDir, "terraform.tfstate"))
	if err != nil {
		t.Fatalf("terraform state show in the generated workspace failed: %v", err)
	}
	if n := len(state.Values.RootModule.Resources); n != expectResCnt {
		t.Fatalf("expected terrafied resource: %d, got=%d", expectResCnt, n)
	}
}
