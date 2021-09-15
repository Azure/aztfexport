package internal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/magodo/aztfy/internal/armtemplate"
	"github.com/magodo/aztfy/schema"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-06-01/resources"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-exec/tfexec"
)

// The minimun required terraform version that has the `terraform add` command.
var minRequiredTFVersion = version.Must(version.NewSemver("1.1.0-alpha20210811"))

type Meta struct {
	subscriptionId string
	resourceGroup  string
	workspace      string
	tf             *tfexec.Terraform
	auth           *Authorizer
	armTemplate    armtemplate.Template
}

func NewMeta(ctx context.Context, rg string) (*Meta, error) {
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
	if err := os.RemoveAll(wsp); err != nil {
		return nil, fmt.Errorf("removing existing workspace %q: %w", wsp, err)
	}
	if err := os.MkdirAll(wsp, 0755); err != nil {
		return nil, fmt.Errorf("creating workspace %q: %w", wsp, err)
	}

	// Authentication
	auth, err := NewAuthorizer()
	if err != nil {
		return nil, fmt.Errorf("building authorizer: %w", err)
	}

	// Initialize the Terraform
	execPath, err := FindTerraform(ctx, tfDir, minRequiredTFVersion)
	if err != nil {
		return nil, fmt.Errorf("error finding a terraform exectuable: %w", err)
	}

	tf, err := tfexec.NewTerraform(wsp, execPath)
	if err != nil {
		return nil, fmt.Errorf("error running NewTerraform: %w", err)
	}

	return &Meta{
		subscriptionId: auth.Config.SubscriptionID,
		resourceGroup:  rg,
		workspace:      wsp,
		tf:             tf,
		auth:           auth,
	}, nil
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

func (meta *Meta) InitProvider(ctx context.Context) error {
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

func (meta *Meta) ExportArmTemplate(ctx context.Context) error {
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

func (meta *Meta) ImportList() ImportList {
	var ids []string
	for _, res := range meta.armTemplate.Resources {
		ids = append(ids, res.ID(meta.subscriptionId, meta.resourceGroup))
	}
	ids = append(ids, armtemplate.ResourceGroupId.ID(meta.subscriptionId, meta.resourceGroup))

	l := make(ImportList, 0, len(ids))
	for _, id := range ids {
		l = append(l, ImportItem{
			ResourceID:     id,
		})
	}
	return l
}

type ImportList []ImportItem

func (l ImportList) NonSkipped() ImportList {
	var out ImportList
	for _, item := range l {
		if item.Skip {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (l ImportList) ImportErrored() ImportList {
	var out ImportList
	for _, item := range l {
		if item.ImportError == nil {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (l ImportList) Imported() ImportList {
	var out ImportList
	for _, item := range l.NonSkipped() {
		if item.ImportError != nil {
			continue
		}
		out = append(out, item)
	}
	return out
}

type ImportItem struct {
	// The azure resource id
	ResourceID string

	// Wether this azure resource should be skipped from importing
	Skip bool

	// Whether this azure resource failed to import into terraform (this might due to the TFResourceType doesn't match the resource)
	ImportError error

	// The terraform resource type
	TFResourceType string

	// The terraform resource name
	TFResourceName string
}

func (item *ImportItem) TFAddr() string {
	if item.Skip {
		return ""
	}
	return item.TFResourceType + "." + item.TFResourceName
}

func (meta *Meta) ResolveImportList(l ImportList) (ImportList, error) {
	tfResourceMap := schema.ProviderSchemaInfo.ResourceSchemas
	var ol ImportList
	// userResourceMap is used to track the resource types and resource names that are specified by users.
	userResourceMap := map[string]map[string]bool{}
	reader := bufio.NewReader(os.Stdin)
	color.Cyan("\nPlease input either the Terraform resource type and name in the form of \"<resource type>.<resource name>\", or simply enter to skip\n")
	for idx, item := range l {
		for {
			fmt.Printf("[%d/%d] %q: ", idx+1, len(l), item.ResourceID)
			input, err := reader.ReadString('\n')
			if err != nil {
				return nil, fmt.Errorf("reading for resource %q: %w", item.ResourceID, err)
			}
			input = strings.TrimSpace(input)
			if input == "" {
				item.Skip = true
				break
			}
			segs := strings.Split(input, ".")
			if len(segs) != 2 {
				fmt.Println(`Invalid input format, should be "<resource type>.<resource name>". Please input again...`)
				continue
			}
			rt, rn := segs[0], segs[1]
			if _, ok := tfResourceMap[rt]; !ok {
				fmt.Printf("Invalid resource type %q. Please input again...\n", rt)
				continue
			}

			rnMap, ok := userResourceMap[rt]
			if !ok {
				rnMap = map[string]bool{}
				userResourceMap[rt] = rnMap
			}
			if _, ok := rnMap[rn]; ok {
				fmt.Printf("There exists a %s with the name %q. Please choose another name...\n", rt, rn)
				continue
			}
			rnMap[rn] = true

			item.TFResourceType = rt
			item.TFResourceName = rn
			break
		}
		ol = append(ol, item)
	}

	return ol, nil
}

func (meta *Meta) Import(ctx context.Context, list ImportList) (ImportList, error) {
	if len(list.NonSkipped()) == 0 {
		return nil, nil
	}

	color.Cyan("\nImport Azure Resources\n")

	// Generate a temp Terraform config to include the empty template for each resource.
	// This is required for the following importing.
	cfgFile := filepath.Join(meta.workspace, "main.tf")

	var tpls []string
	for _, item := range list.NonSkipped() {
		tpl, err := meta.tf.Add(ctx, item.TFAddr())
		if err != nil {
			return nil, fmt.Errorf("generating resource template for %s: %w", item.TFAddr(), err)
		}
		tpls = append(tpls, tpl)
	}
	if err := os.WriteFile(cfgFile, []byte(strings.Join(tpls, "\n")), 0644); err != nil {
		return nil, fmt.Errorf("generating resource template file: %w", err)
	}
	// Remove the temp Terraform config once resources are imported.
	// This is due to the fact that "terraform add" will complain the resource to be added already exist in the config, even we are outputting to stdout.
	// This should be resolved once hashicorp/terraform#29220 is addressed.
	defer os.Remove(cfgFile)

	// Import resources
	out := ImportList{}
	for idx, item := range list.NonSkipped() {
		fmt.Printf("[%d/%d] Importing %q as %s...\n", idx+1, len(list.NonSkipped()), item.ResourceID, item.TFAddr())
		err := meta.tf.Import(ctx, item.TFAddr(), item.ResourceID)
		if err != nil {
			emsg := []string{}
			for _, el := range strings.Split(err.Error(), "\n") {
				el := strings.TrimSpace(el)
				if el == "" {
					continue
				}
				emsg = append(emsg, "\t"+el)
			}
			color.Red(strings.Join(emsg, "\n"))
		}
		item.ImportError = err
		out = append(out, item)
	}

	return out, nil
}

type ConfigInfos []ConfigInfo

type ConfigInfo struct {
	ImportItem
	hcl *hclwrite.File
}

func (cfg ConfigInfo) DumpHCL(w io.Writer) (int, error) {
	return w.Write(hclwrite.Format(cfg.hcl.Bytes()))
}

func (meta *Meta) StateToConfig(ctx context.Context, list ImportList) (ConfigInfos, error) {
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
		tuneHCLSchemaForResource(rb, sch)

		out = append(out, ConfigInfo{
			ImportItem: item,
			hcl:        f,
		})
	}

	return out, nil
}

func (meta *Meta) ResolveDependency(ctx context.Context, configs ConfigInfos) (ConfigInfos, error) {
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

		meta.hclBlockAppendDependency(cfg.hcl.Body().Blocks()[0].Body(), depInfo[armId], configSet)
		out = append(out, cfg)
	}

	return out, nil
}

func (meta *Meta) GenerateConfig(cfgs ConfigInfos) error {
	cfgFile := filepath.Join(meta.workspace, "main.tf")
	buf := bytes.NewBuffer([]byte{})
	for i, cfg := range cfgs {
		cfg.DumpHCL(buf)
		if i != len(cfgs)-1 {
			buf.Write([]byte("\n"))
		}
	}
	if err := os.WriteFile(cfgFile, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("generating main configuration file: %w", err)
	}

	return nil
}

func (meta *Meta) hclBlockAppendDependency(body *hclwrite.Body, armIds []armtemplate.ResourceId, cfgset map[armtemplate.ResourceId]ConfigInfo) error {
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
