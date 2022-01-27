package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/aztfy/internal/armtemplate"
	"github.com/Azure/aztfy/schema"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-06-01/resources"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-exec/tfexec"
)

// The required terraform version that has the `terraform add` command.
var minRequiredTFVersion = version.Must(version.NewSemver("v1.1.0-alpha20210630"))
var maxRequiredTFVersion = version.Must(version.NewSemver("v1.1.0-alpha20211006"))

type MetaImpl struct {
	subscriptionId string
	resourceGroup  string
	workspace      string
	tf             *tfexec.Terraform
	auth           *Authorizer
	armTemplate    armtemplate.Template
}

func newMetaImpl(rg string, outputDir string) (Meta, error) {
	ctx := context.TODO()

	// Initialize the workspace
	cachedir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("error finding the user cache directory: %w", err)
	}

	// Initialize the workspace
	rootDir := filepath.Join(cachedir, "aztfy")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("creating workspace root %q: %w", rootDir, err)
	}

	tfDir := filepath.Join(rootDir, "terraform")
	if err := os.MkdirAll(tfDir, 0755); err != nil {
		return nil, fmt.Errorf("creating terraform cache dir %q: %w", tfDir, err)
	}

	wsp := filepath.Join(rootDir, rg)

	if outputDir != "" {
		wsp = outputDir

		// Ensure wsp is an empty directory
		stat, err := os.Stat(wsp)
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("the output directory %q doesn't exist", wsp)
		}
		if !stat.IsDir() {
			return nil, fmt.Errorf("the output path %q is not a directory", wsp)
		}

		f, err := os.Open(wsp)
		if err != nil {
			return nil, err
		}
		_, err = f.Readdirnames(1) // Or f.Readdir(1)
		f.Close()
		if err != io.EOF {
			return nil, fmt.Errorf("the output directory %q is not empty", wsp)
		}
	} else {
		if err := os.RemoveAll(wsp); err != nil {
			return nil, fmt.Errorf("removing existing workspace %q: %w", wsp, err)
		}
		if err := os.MkdirAll(wsp, 0755); err != nil {
			return nil, fmt.Errorf("creating workspace %q: %w", wsp, err)
		}
	}

	// Authentication
	auth, err := NewAuthorizer()
	if err != nil {
		return nil, fmt.Errorf("building authorizer: %w", err)
	}

	// Initialize the Terraform
	execPath, err := FindTerraform(ctx, tfDir, minRequiredTFVersion, maxRequiredTFVersion)
	if err != nil {
		return nil, fmt.Errorf("error finding a terraform exectuable: %w", err)
	}

	tf, err := tfexec.NewTerraform(wsp, execPath)
	if err != nil {
		return nil, fmt.Errorf("error running NewTerraform: %w", err)
	}

	return &MetaImpl{
		subscriptionId: auth.Config.SubscriptionID,
		resourceGroup:  rg,
		workspace:      wsp,
		tf:             tf,
		auth:           auth,
	}, nil
}

func (meta MetaImpl) ResourceGroupName() string {
	return meta.resourceGroup
}

func (meta MetaImpl) Workspace() string {
	return meta.workspace
}

func (meta *MetaImpl) Init() error {
	ctx := context.TODO()
	if err := meta.initProvider(ctx); err != nil {
		return err
	}
	if err := meta.exportArmTemplate(ctx); err != nil {
		return err
	}
	return nil
}

func (meta MetaImpl) ListResource() ImportList {
	var ids []string
	for _, res := range meta.armTemplate.Resources {
		ids = append(ids, res.ID(meta.subscriptionId, meta.resourceGroup))
	}
	ids = append(ids, armtemplate.ResourceGroupId.ID(meta.subscriptionId, meta.resourceGroup))

	l := make(ImportList, 0, len(ids))
	for i, id := range ids {
		l = append(l, ImportItem{
			ResourceID:     id,
			TFResourceName: fmt.Sprintf("res-%d", i),
		})
	}
	return l
}

func (meta *MetaImpl) CleanTFState(addr string) {
	ctx := context.TODO()
	meta.tf.StateRm(ctx, addr)
}

func (meta MetaImpl) Import(item *ImportItem) {
	ctx := context.TODO()

	// Generate a temp Terraform config to include the empty template for each resource.
	// This is required for the following importing.
	cfgFile := filepath.Join(meta.workspace, "main.tf")
	tpl, err := meta.tf.Add(ctx, item.TFAddr())
	if err != nil {
		item.ImportError = fmt.Errorf("generating resource template for %s: %w", item.TFAddr(), err)
		return
	}
	if err := os.WriteFile(cfgFile, []byte(tpl), 0644); err != nil {
		item.ImportError = fmt.Errorf("generating resource template file: %w", err)
		return
	}
	defer os.Remove(cfgFile)

	// Import resources
	err = meta.tf.Import(ctx, item.TFAddr(), item.ResourceID)
	item.ImportError = err
	item.Imported = err == nil
}

func (meta MetaImpl) GenerateCfg(l ImportList) error {
	ctx := context.TODO()

	cfginfos, err := meta.stateToConfig(ctx, l)
	if err != nil {
		return fmt.Errorf("converting from state to configurations: %w", err)
	}
	cfginfos, err = meta.resolveDependency(cfginfos)
	if err != nil {
		return fmt.Errorf("resolving cross resource dependencies: %w", err)
	}
	return meta.generateConfig(cfginfos)
}

func providerConfig() string {
	return fmt.Sprintf(`terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
      version = "%s"
    }
  }
}

provider "azurerm" {
  features {}
}
`, schema.ProviderVersion)
}

func (meta *MetaImpl) initProvider(ctx context.Context) error {
	cfgFile := filepath.Join(meta.workspace, "provider.tf")

	// Always use the latest provider version here, as this is a one shot tool, which should guarantees to work with the latest version.
	if err := os.WriteFile(cfgFile, []byte(providerConfig()), 0644); err != nil {
		return fmt.Errorf("error creating provider config: %w", err)
	}

	if err := meta.tf.Init(ctx); err != nil {
		return fmt.Errorf("error running terraform init: %s", err)
	}

	return nil
}

func (meta *MetaImpl) exportArmTemplate(ctx context.Context) error {
	client := meta.auth.NewResourceGroupClient()

	exportOpt := "SkipAllParameterization"
	future, err := client.ExportTemplate(ctx, meta.resourceGroup, resources.ExportTemplateRequest{
		ResourcesProperty: &[]string{"*"},
		Options:           &exportOpt,
	})
	if err != nil {
		return fmt.Errorf("exporting arm template of resource group %s: %w", meta.resourceGroup, err)
	}

	if err := future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("waiting for exporting arm template of resource group %s: %w", meta.resourceGroup, err)
	}

	result, err := future.Result(client)
	if err != nil {
		return fmt.Errorf("getting the arm template of resource group %s: %w", meta.resourceGroup, err)
	}

	// The response has been read into the ".Template" field as an interface, and the reader has been drained.
	// As we have defined some (useful) types for the arm template, so we will do a json marshal then unmarshal here
	// to convert the ".Template" (interface{}) into that artificial type.
	raw, err := json.Marshal(result.Template)
	if err != nil {
		return fmt.Errorf("marshalling the template: %w", err)
	}
	if err := json.Unmarshal(raw, &meta.armTemplate); err != nil {
		return fmt.Errorf("unmarshalling the template: %w", err)
	}

	return nil
}

func (meta MetaImpl) stateToConfig(ctx context.Context, list ImportList) (ConfigInfos, error) {
	out := ConfigInfos{}

	for _, item := range list.Imported() {
		tpl, err := meta.tf.Add(ctx, item.TFAddr(), tfexec.FromState(true))
		if err != nil {
			return nil, fmt.Errorf("converting terraform state to config for resource %s: %w", item.TFAddr(), err)
		}
		f, diag := hclwrite.ParseConfig([]byte(tpl), "", hcl.InitialPos)
		if diag.HasErrors() {
			return nil, fmt.Errorf("parsing the HCL generated by \"terraform add\" of %s: %s", item.TFAddr(), diag.Error())
		}

		rb := f.Body().Blocks()[0].Body()
		sch := schema.ProviderSchemaInfo.ResourceSchemas[item.TFResourceType]
		if err := tuneHCLSchemaForResource(rb, sch); err != nil {
			return nil, err
		}

		out = append(out, ConfigInfo{
			ImportItem: item,
			hcl:        f,
		})
	}

	return out, nil
}

func (meta MetaImpl) resolveDependency(configs ConfigInfos) (ConfigInfos, error) {
	depInfo := meta.armTemplate.DependencyInfo()

	configSet := map[armtemplate.ResourceId]ConfigInfo{}
	for _, cfg := range configs {
		armId, err := armtemplate.NewResourceId(cfg.ResourceID)
		if err != nil {
			return nil, fmt.Errorf("new arm tempalte resource id from azure resource id: %w", err)
		}
		configSet[*armId] = cfg
	}

	// Iterate each config to add dependency by querying the dependency info from arm template.
	var out ConfigInfos
	for armId, cfg := range configSet {
		if armId == armtemplate.ResourceGroupId {
			out = append(out, cfg)
			continue
		}
		// This should never happen as we always ensure there is at least one implicit dependency on the resource group for each resource.
		if _, ok := depInfo[armId]; !ok {
			return nil, fmt.Errorf("can't find resource %q in the arm template", armId.ID(meta.subscriptionId, meta.resourceGroup))
		}

		if err := meta.hclBlockAppendDependency(cfg.hcl.Body().Blocks()[0].Body(), depInfo[armId], configSet); err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}

	return out, nil
}

func (meta MetaImpl) hclBlockAppendDependency(body *hclwrite.Body, armIds []armtemplate.ResourceId, cfgset map[armtemplate.ResourceId]ConfigInfo) error {
	dependencies := []string{}
	for _, armid := range armIds {
		cfg, ok := cfgset[armid]
		if !ok {
			dependencies = append(dependencies, fmt.Sprintf("# Depending on %q, which is not imported by Terraform.", armid.ID(meta.subscriptionId, meta.resourceGroup)))
			continue
		}
		dependencies = append(dependencies, cfg.TFAddr()+",")
	}
	if len(dependencies) > 0 {
		src := []byte("depends_on = [\n" + strings.Join(dependencies, "\n") + "\n]")
		expr, diags := hclwrite.ParseConfig(src, "generate_depends_on", hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf(`building "depends_on" attribute: %s`, diags.Error())
		}

		body.SetAttributeRaw("depends_on", expr.Body().GetAttribute("depends_on").Expr().BuildTokens(nil))
	}

	return nil
}

func (meta MetaImpl) generateConfig(cfgs ConfigInfos) error {
	cfgFile := filepath.Join(meta.workspace, "main.tf")
	buf := bytes.NewBuffer([]byte{})
	for i, cfg := range cfgs {
		if _, err := cfg.DumpHCL(buf); err != nil {
			return err
		}
		if i != len(cfgs)-1 {
			buf.Write([]byte("\n"))
		}
	}
	if err := os.WriteFile(cfgFile, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("generating main configuration file: %w", err)
	}

	return nil
}
