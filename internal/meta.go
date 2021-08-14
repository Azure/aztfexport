package internal

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/hashicorp/terraform-schema/earlydecoder"
	"github.com/hashicorp/terraform-schema/module"
)

type Meta struct {
	resourceGroup string
	workspace     string
	tf            *tfexec.Terraform
	auth          *Authorizer
}

func NewMeta(ctx context.Context, rg string) (*Meta, error) {
	// Initialize the workspace
	cachedir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("error finding the user cache directory: %w", err)
	}

	// Initialize the workspace
	rootDir := filepath.Join(cachedir, "tfy")
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
	execPath, err := FindTerraform(ctx, tfDir, version.Must(version.NewVersion("1.1.0")))
	if err != nil {
		return nil, fmt.Errorf("error finding a terraform exectuable: %w", err)
	}

	tf, err := tfexec.NewTerraform(wsp, execPath)
	if err != nil {
		return nil, fmt.Errorf("error running NewTerraform: %w", err)
	}

	return &Meta{
		resourceGroup: rg,
		workspace:     wsp,
		tf:            tf,
		auth:          auth,
	}, nil
}

func (meta *Meta) InitProvider(ctx context.Context) error {
	cfgFile := filepath.Join(meta.workspace, "provider.tf")

	// Always use the latest provider version here, as this is a one shot tool, which should guarantees to work with the latest version.
	if err := os.WriteFile(cfgFile, []byte(`provider "azurerm" {
  features {}
}
`), 0644); err != nil {
		return fmt.Errorf("error creating provider config: %w", err)
	}

	if err := meta.tf.Init(ctx); err != nil {
		return fmt.Errorf("error running terraform init: %s", err)
	}

	return nil
}

func (meta *Meta) ListResources(ctx context.Context) ([]string, error) {
	rgc := meta.auth.NewResourceGroupClient()
	resp, err := rgc.Get(ctx, meta.resourceGroup)
	if err != nil {
		return nil, fmt.Errorf("getting resource group %q: %w", meta.resourceGroup, err)
	}
	if resp.ID == nil || *resp.ID == "" {
		return nil, fmt.Errorf("unexpected nil/empty ID for resource group %q", meta.resourceGroup)
	}

	ids := []string{*resp.ID}

	// Retrieve the IDs of the embedded resources
	rc := meta.auth.NewResourceClient()
	results, err := rc.ListByResourceGroupComplete(ctx, meta.resourceGroup, "", "", nil)
	if err != nil {
		return nil, fmt.Errorf("listing resources in resource group %q: %w", meta.resourceGroup, err)
	}
	for results.NotDone() {
		val := results.Value()
		if val.ID != nil && *val.ID != "" {
			ids = append(ids, *val.ID)
		}

		if err := results.NextWithContext(ctx); err != nil {
			return nil, fmt.Errorf("retrieving next page of nested resources in resource group %q: %w", meta.resourceGroup, err)
		}
	}

	return ids, nil
}

type ImportList []ImportItem

type ImportItem struct {
	ResourceID     string
	Skip           bool
	TFResourceType string
	TFResourceName string
}

func (item *ImportItem) TFAddr() string {
	if item.Skip {
		return ""
	}
	return item.TFResourceType + "." + item.TFResourceName
}

func (meta *Meta) ResolveImportList(ctx context.Context, ids []string) (ImportList, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	schema, err := meta.tf.ProvidersSchema(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting provider schema: %w", err)
	}
	tfResourceMap := schema.Schemas["registry.terraform.io/hashicorp/azurerm"].ResourceSchemas

	var list ImportList
	// userResourceMap is used to track the resource types and resource names that are specified by users.
	userResourceMap := map[string]map[string]bool{}
	reader := bufio.NewReader(os.Stdin)
	fmt.Println(`Please input the Terraform resource type and name for each Azure resource in form of "<resource type>.<resource name>. Press enter with no input will skip importing that resource.`)
	for idx, id := range ids {
		item := ImportItem{
			ResourceID: id,
		}
		for {
			fmt.Printf("[%d/%d] %q: ", idx+1, len(ids), id)
			input, err := reader.ReadString('\n')
			if err != nil {
				return nil, fmt.Errorf("reading for resource %q: %w", id, err)
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
		list = append(list, item)
	}

	return list, nil
}

func (meta *Meta) Import(ctx context.Context, list ImportList) error {
	// Generate a temp Terraform config to include the empty template for each resource.
	// This is required for the following importing.
	cfgFile := filepath.Join(meta.workspace, "main.tf")
	var tpls []string
	for _, item := range list {
		if item.Skip {
			continue
		}
		tpl, err := meta.tf.Add(ctx, item.TFAddr())
		if err != nil {
			return fmt.Errorf("generating resource template for %s: %w", item.TFAddr(), err)
		}
		tpls = append(tpls, tpl)
	}
	if err := os.WriteFile(cfgFile, []byte(strings.Join(tpls, "\n")), 0644); err != nil {
		return fmt.Errorf("generating resource template cfgFile file: %w", err)
	}
	// Remove the temp Terraform config once resources are imported.
	// This is due to the fact that "terraform add" will complain the resource to be added already exist in the config, even we are outputting to stdout.
	// This should be resolved once hashicorp/terraform#29220 is addressed.
	defer os.Remove(cfgFile)

	// Import resources
	for idx, item := range list {
		if item.Skip {
			continue
		}
		fmt.Printf("[%d/%d] Importing %q as %s\n", idx+1, len(list), item.ResourceID, item.TFAddr())
		if err := meta.tf.Import(ctx, item.TFAddr(), item.ResourceID); err != nil {
			return err
		}
	}

	return nil
}

type ConfigInfos []ConfigInfo

type ConfigInfo struct {
	ImportItem
	hcl *hclwrite.File
}

func (meta *Meta) StateToConfig(ctx context.Context, list ImportList) (ConfigInfos, error) {
	out := ConfigInfos{}
	for _, item := range list {
		if item.Skip {
			continue
		}
		tpl, err := meta.tf.Add(ctx, item.TFAddr(), tfexec.FromState(true))
		if err != nil {
			return nil, fmt.Errorf("converting terraform state to config for resource %s: %w", item.TFAddr(), err)
		}
		f, diag := hclwrite.ParseConfig([]byte(tpl), "", hcl.InitialPos)
		if diag.HasErrors() {
			return nil, fmt.Errorf("parsing the HCL generated by \"terraform add\" of %s: %s", item.TFAddr(), diag.Error())
		}

		// Currently, some special attributes are output by `terraform add`, while should be excluded.
		// This is tracked in: https://github.com/hashicorp/terraform/issues/29219
		// We are manually excluding these special properties here by modifying the hcl.
		rb := f.Body().Blocks()[0].Body()
		rb.RemoveAttribute("id")
		rb.RemoveBlock(rb.FirstMatchingBlock("timeouts", nil))

		out = append(out, ConfigInfo{
			ImportItem: item,
			hcl:        f,
		})
	}

	// var buf strings.Builder
	// for _, cfg := range out {
	// 	if _, err := cfg.hcl.WriteTo(&buf); err != nil {
	// 		return nil, err
	// 	}
	// }
	// fmt.Println(buf.String())

	return out, nil
}

func tfschemaModule(filename string, b []byte) (*module.Meta, error) {
	f, diags := hclsyntax.ParseConfig(b, filename, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing HCL for %q: %s", filename, diags.Error())
	}
	meta, diags := earlydecoder.LoadModule("root", map[string]*hcl.File{
		filename: f,
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("loading module: %s", diags.Error())
	}
	return meta, nil
}
