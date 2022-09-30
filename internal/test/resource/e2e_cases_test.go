package resource

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Azure/aztfy/internal"
	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/test"
	"github.com/Azure/aztfy/internal/test/cases"
	"github.com/Azure/aztfy/internal/utils"
	"github.com/hashicorp/terraform-exec/tfexec"
)

func runCase(t *testing.T, d test.Data, c cases.Case) {
	tfexecPath := test.EnsureTF(t)

	provisionDir := t.TempDir()
	if test.Keep() {
		provisionDir, _ = os.MkdirTemp("", "")
		t.Log(provisionDir)
	}

	os.Chdir(provisionDir)
	if err := utils.WriteFileSync("main.tf", []byte(c.Tpl(d)), 0644); err != nil {
		t.Fatalf("created to create the TF config file: %v", err)
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

	aztfyDir := t.TempDir()
	l, err := c.SingleResourceContext(d)
	if err != nil {
		t.Fatalf("failed to get resource ids: %v", err)
	}
	for idx, rctx := range l {
		cfg := config.Config{
			CommonConfig: config.CommonConfig{
				SubscriptionId: os.Getenv("ARM_SUBSCRIPTION_ID"),
				OutputDir:      aztfyDir,
				BackendType:    "local",
				DevProvider:    true,
				PlainUI:        true,
				Overwrite:      true,
			},
			ResourceId:     rctx.AzureId,
			TFResourceName: fmt.Sprintf("res-%d", idx),
		}
		t.Logf("Resource importing %s\n", rctx.AzureId)
		if err := internal.BatchImport(cfg); err != nil {
			t.Fatalf("failed to run resource import: %v", err)
		}
		test.Verify(t, ctx, aztfyDir, tfexecPath, rctx.ExpectResourceCount)
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
