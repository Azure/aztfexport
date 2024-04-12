package resourcegroup

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/aztfexport/internal"
	internalconfig "github.com/Azure/aztfexport/internal/config"
	"github.com/Azure/aztfexport/internal/test"
	"github.com/Azure/aztfexport/internal/test/cases"
	"github.com/Azure/aztfexport/pkg/config"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/magodo/terraform-client-go/tfclient"
)

func runHCLOnly(t *testing.T, d test.Data, c cases.Case) {
	if os.Getenv(test.TestPluginPathEnvVar) == "" {
		t.Skipf("Skipping as %q not defined", test.TestPluginPathEnvVar)
	}
	tfexecPath := test.EnsureTF(t)

	// #nosec G204
	tfc, err := tfclient.New(tfclient.Option{
		Cmd:    exec.Command(os.Getenv(test.TestPluginPathEnvVar)),
		Logger: hclog.NewNullLogger(),
	})
	if err != nil {
		t.Fatalf("new tfclient: %v", err)
	}

	provisionDir := t.TempDir()
	if test.Keep() {
		provisionDir, _ = os.MkdirTemp("", "")
		t.Log(provisionDir)
	}

	if err := os.WriteFile(filepath.Join(provisionDir, "main.tf"), []byte(c.Tpl(d)), 0644); err != nil {
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

	aztfexportDir := t.TempDir()
	outDir1 := filepath.Join(aztfexportDir, "1")
	outDir2 := filepath.Join(aztfexportDir, "2")

	if err := os.MkdirAll(outDir1, 0750); err != nil {
		log.Fatalf("creating %s: %v", outDir1, err)
	}
	if err := os.MkdirAll(outDir2, 0750); err != nil {
		log.Fatalf("creating %s: %v", outDir2, err)
	}

	cred, clientOpt := test.BuildCredAndClientOpt(t)

	cfg := internalconfig.NonInteractiveModeConfig{
		Config: config.Config{
			CommonConfig: config.CommonConfig{
				SubscriptionId:       os.Getenv("ARM_SUBSCRIPTION_ID"),
				AzureSDKCredential:   cred,
				AzureSDKClientOption: *clientOpt,
				OutputDir:            outDir1,
				BackendType:          "local",
				DevProvider:          true,
				Parallelism:          10,
				HCLOnly:              true,
				ProviderName:         "azurerm",
			},
			ResourceGroupName:   d.RandomRgName(),
			ResourceNamePattern: "res-",
		},
		PlainUI: true,
	}
	t.Logf("Batch importing the resource group (with terraform) %s\n", d.RandomRgName())
	if err := internal.BatchImport(ctx, cfg); err != nil {
		t.Fatalf("failed to run batch import (with terraform): %v", err)
	}

	cfg.Config.CommonConfig.OutputDir = outDir2
	cfg.Config.CommonConfig.TFClient = tfc
	t.Logf("Batch importing the resource group (with tfclient) %s\n", d.RandomRgName())
	if err := internal.BatchImport(ctx, cfg); err != nil {
		t.Fatalf("failed to run batch import (with tfclient): %v", err)
	}

	// Compare the main.tf files generated in two runs are the same
	main1, main2 := filepath.Join(outDir1, "main.tf"), filepath.Join(outDir2, "main.tf")
	f1, err := os.ReadFile(main1)
	if err != nil {
		t.Fatalf("reading %s: %v", main1, err)
	}
	f2, err := os.ReadFile(main2)
	if err != nil {
		t.Fatalf("reading %s: %v", main2, err)
	}

	edits := myers.ComputeEdits(span.URIFromPath(main1), string(f1), string(f2))
	if len(edits) != 0 {
		changes := fmt.Sprint(gotextdiff.ToUnified(main1, main2, string(f1), edits))
		t.Fatalf("main.tf file between two runs have diff: %s", changes)
	}
}

func TestComputeVMDiskForHCLOnly(t *testing.T) {
	t.Parallel()
	test.Precheck(t)
	c, d := cases.CaseComputeVMDisk{}, test.NewData()
	runHCLOnly(t, d, c)
}
