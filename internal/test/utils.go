package test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"text/template"

	"github.com/Azure/aztfy/internal/resmap"
	"github.com/hashicorp/go-version"
	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/src"
	"github.com/hashicorp/terraform-exec/tfexec"
)

const TestToggleEnvVar = "AZTFY_E2E"

func Keep() bool {
	return os.Getenv("AZTFY_KEEP") != ""
}

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

	planFile := filepath.Join(t.TempDir(), "plan")
	diff, err := tf.Plan(ctx, tfexec.Out(planFile))
	if err != nil {
		t.Fatalf("terraform plan in the generated workspace failed: %v", err)
	}
	if diff {
		plan, err := tf.ShowPlanFile(ctx, planFile)
		if err != nil {
			t.Logf("failed to show plan file %s: %v", planFile, err)
		} else {
			for _, change := range plan.ResourceChanges {
				if change == nil {
					continue
				}
				b, err := json.MarshalIndent(change.Change, "", "  ")
				if err != nil {
					t.Logf("failed to marshal plan for %s: %v", change.Address, err)
				} else {
					t.Logf("%s\n%s\n", change.Address, string(b))
				}
			}
		}
		t.Fatalf("terraform plan has diff")
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

func ResourceMapping(tpl string) (resmap.ResourceMapping, error) {
	funcMap := template.FuncMap{
		"ToUpper": strings.ToUpper,
		"Quote":   strconv.Quote,
	}

	gotpl, err := template.New("myTemplate").Funcs(funcMap).Parse(tpl)
	if err != nil {
		return nil, err
	}

	var result bytes.Buffer
	if err := gotpl.Execute(&result, nil); err != nil {
		return nil, err
	}

	m := resmap.ResourceMapping{}
	if err := json.Unmarshal(result.Bytes(), &m); err != nil {
		return nil, err
	}

	return m, nil
}
