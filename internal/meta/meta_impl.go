package meta

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/aztfy/internal/client"

	"github.com/Azure/aztfy/internal/armtemplate"
	"github.com/Azure/aztfy/internal/resmap"
	"github.com/Azure/aztfy/internal/tfaddr"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/magodo/tfadd/providers/azurerm"
	"github.com/magodo/tfadd/tfadd"
)

type MetaImpl struct {
	subscriptionId string
	resourceGroup  string
	rootdir        string
	outdir         string
	tf             *tfexec.Terraform
	clientBuilder  *client.ClientBuilder
	armTemplate    armtemplate.FQTemplate

	// Key is azure resource id; Value is terraform resource addr.
	// For azure resources not in this mapping, they are all initialized as to skip.
	resourceMapping resmap.ResourceMapping

	resourceNamePrefix string
	resourceNameSuffix string

	backendType   string
	backendConfig []string

	// Use a safer name which is less likely to conflicts with users' existing files.
	// This is mainly used for the --append option.
	useSafeFilename bool
}

func newMetaImpl(cfg config.Config) (Meta, error) {
	// Initialize the rootdir
	cachedir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("error finding the user cache directory: %w", err)
	}
	rootdir := filepath.Join(cachedir, "aztfy")
	if err := os.MkdirAll(rootdir, 0755); err != nil {
		return nil, fmt.Errorf("creating rootdir %q: %w", rootdir, err)
	}

	var outdir string
	if cfg.OutputDir == "" {
		outdir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	} else {
		outdir, err = filepath.Abs(cfg.OutputDir)
		if err != nil {
			return nil, err
		}
	}
	empty, err := dirIsEmpty(outdir)
	if err != nil {
		return nil, err
	}
	if !empty {
		if !cfg.Append {
			if cfg.Overwrite {
				if err := removeEverythingUnder(outdir); err != nil {
					return nil, err
				}
			} else {
				if cfg.BatchMode {
					return nil, fmt.Errorf("the output directory %q is not empty", outdir)
				}

				// Interactive mode
				fmt.Printf("The output directory is not empty - overwrite (Y/N)? ")
				var ans string
				fmt.Scanf("%s", &ans)
				if !strings.EqualFold(ans, "y") {
					return nil, fmt.Errorf("the output directory %q is not empty", outdir)
				} else {
					if err := removeEverythingUnder(outdir); err != nil {
						return nil, err
					}
				}
			}
		}
	}

	// Construct client builder
	b, err := client.NewClientBuilder()
	if err != nil {
		return nil, fmt.Errorf("building authorizer: %w", err)
	}

	// AzureRM provider will honor env.var "AZURE_HTTP_USER_AGENT" when constructing for HTTP "User-Agent" header.
	os.Setenv("AZURE_HTTP_USER_AGENT", "aztfy")

	meta := &MetaImpl{
		subscriptionId:  cfg.SubscriptionId,
		resourceGroup:   cfg.ResourceGroupName,
		rootdir:         rootdir,
		outdir:          outdir,
		clientBuilder:   b,
		resourceMapping: cfg.ResourceMapping,
		backendType:     cfg.BackendType,
		backendConfig:   cfg.BackendConfig,
		useSafeFilename: cfg.Append,
	}

	if pos := strings.LastIndex(cfg.ResourceNamePattern, "*"); pos != -1 {
		meta.resourceNamePrefix, meta.resourceNameSuffix = cfg.ResourceNamePattern[:pos], cfg.ResourceNamePattern[pos+1:]
	} else {
		meta.resourceNamePrefix = cfg.ResourceNamePattern
	}

	return meta, nil
}

func (meta MetaImpl) ResourceGroupName() string {
	return meta.resourceGroup
}

func (meta MetaImpl) Workspace() string {
	return meta.outdir
}

func (meta *MetaImpl) Init() error {
	ctx := context.TODO()

	// Initialize the Terraform
	tfDir := filepath.Join(meta.rootdir, "terraform")
	if err := os.MkdirAll(tfDir, 0755); err != nil {
		return fmt.Errorf("creating terraform cache dir %q: %w", tfDir, err)
	}
	execPath, err := FindTerraform(ctx, tfDir)
	if err != nil {
		return fmt.Errorf("error finding a terraform exectuable: %w", err)
	}
	tf, err := tfexec.NewTerraform(meta.outdir, execPath)
	if err != nil {
		return fmt.Errorf("error running NewTerraform: %w", err)
	}
	meta.tf = tf

	// Initialize the provider
	if err := meta.initProvider(ctx); err != nil {
		return err
	}
	return nil
}

func (meta *MetaImpl) ListResource() (ImportList, error) {
	ctx := context.TODO()

	if err := meta.exportArmTemplate(ctx); err != nil {
		return nil, err
	}

	var ids []string
	for _, res := range meta.armTemplate.Resources {
		ids = append(ids, res.Id)
	}
	rgid, _ := armtemplate.ResourceGroupId.ProviderId(meta.subscriptionId, meta.resourceGroup, nil)
	ids = append(ids, rgid)

	var l ImportList

	for i, id := range ids {
		recommendations := RecommendationsForId(id)
		item := ImportItem{
			ResourceID: id,
			TFAddr: tfaddr.TFAddr{
				Type: "",
				Name: fmt.Sprintf("%s%d%s", meta.resourceNamePrefix, i, meta.resourceNameSuffix),
			},
			Recommendations: recommendations,
		}

		if len(meta.resourceMapping) != 0 {
			if addr, ok := meta.resourceMapping[id]; ok {
				item.TFAddr = addr
			}
		} else {
			// Only auto deduce the TF resource type from recommendations when there is no resource mapping file specified.
			if len(recommendations) == 1 {
				item.TFAddr.Type = recommendations[0]
				item.IsRecommended = true
			}
		}
		l = append(l, item)
	}
	return l, nil
}

func (meta *MetaImpl) CleanTFState(addr string) {
	ctx := context.TODO()
	meta.tf.StateRm(ctx, addr)
}

func (meta MetaImpl) Import(item *ImportItem) {
	ctx := context.TODO()

	// Generate a temp Terraform config to include the empty template for each resource.
	// This is required for the following importing.
	cfgFile := filepath.Join(meta.outdir, meta.filenameTmpCfg())
	tpl := fmt.Sprintf(`resource "%s" "%s" {}`, item.TFAddr.Type, item.TFAddr.Name)
	if err := os.WriteFile(cfgFile, []byte(tpl), 0644); err != nil {
		item.ImportError = fmt.Errorf("generating resource template file: %w", err)
		return
	}
	defer os.Remove(cfgFile)

	// Import resources
	err := meta.tf.Import(ctx, item.TFAddr.String(), item.ResourceID)
	item.ImportError = err
	item.Imported = err == nil
}

func (meta MetaImpl) GenerateCfg(l ImportList) error {
	ctx := context.TODO()

	cfginfos, err := meta.stateToConfig(ctx, l)
	if err != nil {
		return fmt.Errorf("converting from state to configurations: %w", err)
	}
	cfginfos, err = meta.terraformMetaHook(cfginfos)
	if err != nil {
		return fmt.Errorf("Terraform HCL meta hook: %w", err)
	}

	return meta.generateConfig(cfginfos)
}

func (meta MetaImpl) ExportResourceMapping(l ImportList) error {
	m := resmap.ResourceMapping{}
	for _, item := range l {
		m[item.ResourceID] = item.TFAddr
	}
	output := filepath.Join(meta.outdir, ResourceMappingFileName)
	b, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return fmt.Errorf("JSON marshalling the resource mapping: %v", err)
	}
	if err := os.WriteFile(output, b, 0644); err != nil {
		return fmt.Errorf("writing the resource mapping to %s: %v", output, err)
	}
	return nil
}

func (meta *MetaImpl) providerConfig() string {
	return fmt.Sprintf(`terraform {
  backend %q {}
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
`, meta.backendType, azurerm.ProviderSchemaInfo.Version)
}

func (meta MetaImpl) filenameProviderSetting() string {
	if meta.useSafeFilename {
		return "provider.aztfy.tf"
	}
	return "provider.tf"
}

func (meta MetaImpl) filenameMainCfg() string {
	if meta.useSafeFilename {
		return "main.aztfy.tf"
	}
	return "main.tf"
}

func (meta MetaImpl) filenameTmpCfg() string {
	return "tmp.aztfy.tf"
}

func (meta *MetaImpl) initProvider(ctx context.Context) error {
	empty, err := dirIsEmpty(meta.outdir)
	if err != nil {
		return err
	}

	// If the directory is empty, generate the full config as the output directory is empty.
	// Otherwise:
	// - If the output directory already exists the `azurerm` provider setting, then do nothing
	// - Otherwise, just generate the `azurerm` provider setting (as it is only for local backend)
	if empty {
		cfgFile := filepath.Join(meta.outdir, meta.filenameProviderSetting())
		if err := os.WriteFile(cfgFile, []byte(meta.providerConfig()), 0644); err != nil {
			return fmt.Errorf("error creating provider config: %w", err)
		}

		var opts []tfexec.InitOption
		for _, opt := range meta.backendConfig {
			opts = append(opts, tfexec.BackendConfig(opt))
		}
		if err := meta.tf.Init(ctx, opts...); err != nil {
			return fmt.Errorf("error running terraform init: %s", err)
		}
		return nil
	}

	exists, err := dirContainsProviderSetting(meta.outdir)
	if err != nil {
		return err
	}
	if !exists {
		if err := appendToFile(meta.filenameProviderSetting(), `provider "azurerm" {
  features {}
}
`); err != nil {
			return fmt.Errorf("error creating provider config: %w", err)
		}
	}

	return nil
}

func (meta *MetaImpl) exportArmTemplate(ctx context.Context) error {
	client, err := meta.clientBuilder.NewResourceGroupClient(meta.subscriptionId)
	if err != nil {
		return fmt.Errorf("building resource group client: %v", err)
	}

	exportOpt := "SkipAllParameterization"
	resourceOpt := "*"
	poller, err := client.BeginExportTemplate(ctx, meta.resourceGroup, armresources.ExportTemplateRequest{
		Resources: []*string{&resourceOpt},
		Options:   &exportOpt,
	}, nil)
	if err != nil {
		return fmt.Errorf("exporting arm template of resource group %s: %w", meta.resourceGroup, err)
	}
	resp, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: 10 * time.Second})
	if err != nil {
		return fmt.Errorf("waiting for exporting arm template of resource group %s: %w", meta.resourceGroup, err)
	}

	// The response has been read into the ".Template" field as an interface, and the reader has been drained.
	// As we have defined some (useful) types for the arm template, so we will do a json marshal then unmarshal here
	// to convert the ".Template" (interface{}) into that artificial type.
	raw, err := json.Marshal(resp.ResourceGroupExportResult.Template)
	if err != nil {
		return fmt.Errorf("marshalling the template: %w", err)
	}
	var tpl armtemplate.Template
	if err := json.Unmarshal(raw, &tpl); err != nil {
		return fmt.Errorf("unmarshalling the template: %w", err)
	}
	if err := tpl.TweakResources(); err != nil {
		return fmt.Errorf("populating managed resources in the ARM template: %v", err)
	}

	fqTpl, err := tpl.Qualify(meta.subscriptionId, meta.resourceGroup, meta.clientBuilder)
	if err != nil {
		return err
	}
	meta.armTemplate = *fqTpl

	return nil
}

func (meta MetaImpl) stateToConfig(ctx context.Context, list ImportList) (ConfigInfos, error) {
	out := ConfigInfos{}
	for _, item := range list.Imported() {
		b, err := tfadd.State(ctx, meta.tf, tfadd.Target(item.TFAddr.String()))
		if err != nil {
			return nil, fmt.Errorf("converting terraform state to config for resource %s: %w", item.TFAddr, err)
		}
		tpl := meta.cleanupTerraformAdd(string(b))
		f, diag := hclwrite.ParseConfig([]byte(tpl), "", hcl.InitialPos)
		if diag.HasErrors() {
			return nil, fmt.Errorf("parsing the HCL generated by \"terraform add\" of %s: %s", item.TFAddr, diag.Error())
		}

		out = append(out, ConfigInfo{
			ImportItem: item,
			hcl:        f,
		})
	}

	return out, nil
}

func (meta MetaImpl) terraformMetaHook(configs ConfigInfos) (ConfigInfos, error) {
	var err error
	configs, err = meta.resolveDependency(configs)
	if err != nil {
		return nil, fmt.Errorf("resolving cross resource dependencies: %w", err)
	}
	configs, err = meta.lifecycleAddon(configs)
	if err != nil {
		return nil, fmt.Errorf("adding terraform lifecycle: %w", err)
	}
	return configs, nil
}

func (meta MetaImpl) resolveDependency(configs ConfigInfos) (ConfigInfos, error) {
	depInfo, err := meta.armTemplate.DependencyInfo()
	if err != nil {
		return nil, err
	}

	configSet := map[string]ConfigInfo{}
	for _, cfg := range configs {
		configSet[cfg.ResourceID] = cfg
	}

	// Iterate each config to add dependency by querying the dependency info from arm template.
	var out ConfigInfos
	rgid, _ := armtemplate.ResourceGroupId.ProviderId(meta.subscriptionId, meta.resourceGroup, nil)
	for id, cfg := range configSet {
		if id == rgid {
			out = append(out, cfg)
			continue
		}
		// This should never happen as we always ensure there is at least one implicit dependency on the resource group for each resource.
		if _, ok := depInfo[id]; !ok {
			return nil, fmt.Errorf("can't find resource %q in the arm template", id)
		}

		if err := hclBlockAppendDependency(cfg.hcl.Body().Blocks()[0].Body(), depInfo[id], configSet); err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}

	return out, nil
}

func (meta MetaImpl) generateConfig(cfgs ConfigInfos) error {
	cfgFile := filepath.Join(meta.outdir, meta.filenameMainCfg())
	buf := bytes.NewBuffer([]byte{})
	for _, cfg := range cfgs {
		if _, err := cfg.DumpHCL(buf); err != nil {
			return err
		}
		buf.Write([]byte("\n"))
	}
	if err := appendToFile(cfgFile, buf.String()); err != nil {
		return fmt.Errorf("generating main configuration file: %w", err)
	}

	return nil
}

func (meta MetaImpl) cleanupTerraformAdd(tpl string) string {
	segs := strings.Split(tpl, "\n")
	// Removing:
	// - preceding/trailing state lock related log when TF backend is used.
	for len(segs) != 0 && strings.HasPrefix(segs[0], "Acquiring state lock.") {
		segs = segs[1:]
	}

	last := len(segs) - 1
	for last != -1 && (segs[last] == "" || strings.HasPrefix(segs[last], "Releasing state lock.")) {
		segs = segs[:last]
		last = len(segs) - 1
	}
	return strings.Join(segs, "\n")
}

func removeEverythingUnder(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %v", path, err)
	}
	entries, _ := dir.Readdirnames(0)
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(path, entry)); err != nil {
			return fmt.Errorf("failed to remove %s: %v", entry, err)
		}
	}
	dir.Close()
	return nil
}

func dirIsEmpty(path string) (bool, error) {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, fmt.Errorf("the path %q doesn't exist", path)
	}
	if !stat.IsDir() {
		return false, fmt.Errorf("the path %q is not a directory", path)
	}
	dir, err := os.Open(path)
	if err != nil {
		return false, err
	}
	_, err = dir.Readdirnames(1)
	if err != nil {
		if err == io.EOF {
			dir.Close()
			return true, nil
		}
		return false, err
	}
	dir.Close()
	return false, nil
}

func dirContainsProviderSetting(path string) (bool, error) {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, fmt.Errorf("the path %q doesn't exist", path)
	}
	if !stat.IsDir() {
		return false, fmt.Errorf("the path %q is not a directory", path)
	}
	dir, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer dir.Close()

	fnames, err := dir.Readdirnames(0)
	if err != nil {
		return false, err
	}

	// Ideally, we shall use hclgrep for a perfect match. But as the provider setting is simple enough, we do a text matching here.
	p := regexp.MustCompile(`^\s*provider\s+"azurerm"\s*{\s*$`)
	for _, fname := range fnames {
		// fmt.Println(fname)
		// fmt.Println(filepath.Ext(fname))
		if filepath.Ext(fname) != ".tf" {
			continue
		}
		f, err := os.Open(filepath.Join(path, fname))
		if err != nil {
			return false, fmt.Errorf("openning %s: %v", fname, err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if p.MatchString(scanner.Text()) {
				return true, nil
			}
		}
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("reading file %s: %v", fname, err)
		}
		f.Close()
	}
	return false, nil
}

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}
