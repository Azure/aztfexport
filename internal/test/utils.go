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
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
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

func BuildCredAndClientOpt(t *testing.T) (azcore.TokenCredential, *arm.ClientOptions) {
	env := "public"
	if v := os.Getenv("ARM_ENVIRONMENT"); v != "" {
		env = v
	}

	var cloudCfg cloud.Configuration
	switch strings.ToLower(env) {
	case "public":
		cloudCfg = cloud.AzurePublic
	case "usgovernment":
		cloudCfg = cloud.AzureGovernment
	case "china":
		cloudCfg = cloud.AzureChina
	default:
		t.Fatalf("unknown environment specified: %q", env)
	}

	os.Setenv("AZURE_TENANT_ID", os.Getenv("ARM_TENANT_ID"))
	os.Setenv("AZURE_CLIENT_ID", os.Getenv("ARM_CLIENT_ID"))
	os.Setenv("AZURE_CLIENT_SECRET", os.Getenv("ARM_CLIENT_SECRET"))
	os.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", os.Getenv("ARM_CLIENT_CERTIFICATE_PATH"))

	clientOpt := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloudCfg,
			Telemetry: policy.TelemetryOptions{
				ApplicationID: "aztfy",
				Disabled:      false,
			},
			Logging: policy.LogOptions{
				IncludeBody: true,
			},
		},
	}

	cred, err := azidentity.NewClientSecretCredential(
		os.Getenv("ARM_TENANT_ID"),
		os.Getenv("ARM_CLIENT_ID"),
		os.Getenv("ARM_CLIENT_SECRET"),
		&azidentity.ClientSecretCredentialOptions{
			ClientOptions: clientOpt.ClientOptions,
		},
	)
	if err != nil {
		t.Fatalf("failed to obtain a credential: %v", err)
	}

	return cred, clientOpt
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
				before, _ := json.MarshalIndent(change.Change.Before, "", "  ")
				after, _ := json.MarshalIndent(change.Change.After, "", "  ")
				if string(before) == string(after) {
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
