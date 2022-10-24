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

	"github.com/Azure/aztfy/internal/client"
	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/resmap"
	"github.com/Azure/aztfy/internal/utils"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/magodo/tfadd/providers/azurerm"
	"github.com/magodo/tfadd/tfadd"
)

const ResourceMappingFileName = "aztfyResourceMapping.json"
const SkippedResourcesFileName = "aztfySkippedResources.txt"

type BaseMeta interface {
	Init() error
	Workspace() string
	Import(item *ImportItem)
	CleanTFState(addr string)
	GenerateCfg(ImportList) error
	ExportSkippedResources(l ImportList) error
	ExportResourceMapping(ImportList) error
	CleanUpWorkspace() error
}

var _ BaseMeta = &baseMeta{}

type baseMeta struct {
	subscriptionId string
	rootdir        string
	outdir         string
	tf             *tfexec.Terraform
	resourceClient *armresources.Client
	devProvider    bool
	backendType    string
	backendConfig  []string
	fullConfig     bool
	parallelism    int
	hclOnly        bool

	// Use a safer name which is less likely to conflicts with users' existing files.
	// This is mainly used for the --append option.
	useSafeFilename bool

	empty bool
}

func NewBaseMeta(cfg config.CommonConfig) (*baseMeta, error) {
	// Initialize the rootdir
	cachedir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("error finding the user cache directory: %w", err)
	}
	rootdir := filepath.Join(cachedir, "aztfy")
	// #nosec G301
	if err := os.MkdirAll(rootdir, 0750); err != nil {
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
				empty = true
			} else {
				if cfg.Batch {
					return nil, fmt.Errorf("the output directory %q is not empty", outdir)
				}

				// Interactive mode
				fmt.Printf(`
The output directory is not empty. Please choose one of actions below:

* Press "Y" to overwrite the existing directory with new files
* Press "N" to append new files and add to the existing state instead
* Press other keys to quit

> `)
				var ans string
				// #nosec G104
				fmt.Scanf("%s", &ans)
				switch strings.ToLower(ans) {
				case "y":
					if err := removeEverythingUnder(outdir); err != nil {
						return nil, err
					}
					empty = true
				case "n":
					cfg.Append = true
				default:
					return nil, fmt.Errorf("the output directory %q is not empty", outdir)
				}
			}
		}
	}

	// Construct client builder
	b, err := client.NewClientBuilder()
	if err != nil {
		return nil, fmt.Errorf("building authorizer: %w", err)
	}
	resClient, err := b.NewResourcesClient(cfg.SubscriptionId)
	if err != nil {
		return nil, fmt.Errorf("new resource client")
	}

	// AzureRM provider will honor env.var "AZURE_HTTP_USER_AGENT" when constructing for HTTP "User-Agent" header.
	// #nosec G104
	os.Setenv("AZURE_HTTP_USER_AGENT", "aztfy")

	// Avoid the AzureRM provider to call the expensive RP listing API, repeatedly.
	// #nosec G104
	os.Setenv("ARM_PROVIDER_ENHANCED_VALIDATION", "false")
	// #nosec G104
	os.Setenv("ARM_SKIP_PROVIDER_REGISTRATION", "true")

	meta := &baseMeta{
		subscriptionId:  cfg.SubscriptionId,
		rootdir:         rootdir,
		outdir:          outdir,
		resourceClient:  resClient,
		devProvider:     cfg.DevProvider,
		backendType:     cfg.BackendType,
		backendConfig:   cfg.BackendConfig,
		fullConfig:      cfg.FullConfig,
		parallelism:     cfg.Parallelism,
		useSafeFilename: cfg.Append,
		empty:           empty,
		hclOnly:         cfg.HCLOnly,
	}

	return meta, nil
}

func (meta baseMeta) Workspace() string {
	return meta.outdir
}

func (meta *baseMeta) Init() error {
	ctx := context.TODO()

	// Initialize the Terraform
	tfDir := filepath.Join(meta.rootdir, "terraform")
	// #nosec G301
	if err := os.MkdirAll(tfDir, 0750); err != nil {
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
	if v, ok := os.LookupEnv("TF_LOG_PATH"); ok {
		// #nosec G104
		tf.SetLogPath(v)
	}
	if v, ok := os.LookupEnv("TF_LOG"); ok {
		// #nosec G104
		tf.SetLog(v)
	}
	meta.tf = tf

	// Initialize the provider
	if err := meta.initProvider(ctx); err != nil {
		return err
	}
	return nil
}

func (meta *baseMeta) CleanTFState(addr string) {
	ctx := context.TODO()
	// #nosec G104
	meta.tf.StateRm(ctx, addr)
}

func (meta baseMeta) Import(item *ImportItem) {
	ctx := context.TODO()

	// Generate a temp Terraform config to include the empty template for each resource.
	// This is required for the following importing.
	cfgFile := filepath.Join(meta.outdir, meta.filenameTmpCfg())
	tpl := fmt.Sprintf(`resource "%s" "%s" {}`, item.TFAddr.Type, item.TFAddr.Name)
	if err := utils.WriteFileSync(cfgFile, []byte(tpl), 0644); err != nil {
		item.ImportError = fmt.Errorf("generating resource template file: %w", err)
		return
	}
	defer os.Remove(cfgFile)

	// Import resources
	err := meta.tf.Import(ctx, item.TFAddr.String(), item.TFResourceId)
	item.ImportError = err
	item.Imported = err == nil
}

func (meta baseMeta) GenerateCfg(l ImportList) error {
	return meta.generateCfg(l, meta.lifecycleAddon, meta.addDependency)
}

func (meta baseMeta) ExportResourceMapping(l ImportList) error {
	m := resmap.ResourceMapping{}
	for _, item := range l {
		if item.Skip() {
			continue
		}
		m[item.AzureResourceID.String()] = resmap.ResourceMapEntity{
			ResourceId:   item.TFResourceId,
			ResourceType: item.TFAddr.Type,
			ResourceName: item.TFAddr.Name,
		}
	}
	output := filepath.Join(meta.Workspace(), ResourceMappingFileName)
	b, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return fmt.Errorf("JSON marshalling the resource mapping: %v", err)
	}
	if err := os.WriteFile(output, b, 0600); err != nil {
		return fmt.Errorf("writing the resource mapping to %s: %v", output, err)
	}
	return nil
}

func (meta baseMeta) ExportSkippedResources(l ImportList) error {
	var sl []string
	for _, item := range l {
		if item.Skip() {
			sl = append(sl, "- "+item.AzureResourceID.String())
		}
	}
	if len(sl) == 0 {
		return nil
	}

	output := filepath.Join(meta.Workspace(), SkippedResourcesFileName)
	if err := os.WriteFile(output, []byte(fmt.Sprintf(`Following resources are marked to be skipped:

%s
`, strings.Join(sl, "\n"))), 0600); err != nil {
		return fmt.Errorf("writing the skipped resources to %s: %v", output, err)
	}
	return nil
}

func (meta baseMeta) CleanUpWorkspace() error {
	// Do nothing if not HCL only... Otherwise, clean up the workspace to only keep the HCL files.
	if !meta.hclOnly {
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tmpMainCfg := filepath.Join(tmpDir, meta.filenameMainCfg())
	tmpProviderCfg := filepath.Join(tmpDir, meta.filenameProviderSetting())

	if err := utils.CopyFile(filepath.Join(meta.Workspace(), meta.filenameMainCfg()), tmpMainCfg); err != nil {
		return err
	}
	if err := utils.CopyFile(filepath.Join(meta.Workspace(), meta.filenameProviderSetting()), tmpProviderCfg); err != nil {
		return err
	}

	if err := removeEverythingUnder(meta.Workspace()); err != nil {
		return err
	}

	if err := utils.CopyFile(tmpMainCfg, filepath.Join(meta.Workspace(), meta.filenameMainCfg())); err != nil {
		return err
	}
	if err := utils.CopyFile(tmpProviderCfg, filepath.Join(meta.Workspace(), meta.filenameProviderSetting())); err != nil {
		return err
	}

	return nil
}

func (meta baseMeta) generateCfg(l ImportList, cfgTrans ...TFConfigTransformer) error {
	ctx := context.TODO()

	cfginfos, err := meta.stateToConfig(ctx, l)
	if err != nil {
		return fmt.Errorf("converting from state to configurations: %w", err)
	}
	cfginfos, err = meta.terraformMetaHook(cfginfos, cfgTrans...)
	if err != nil {
		return fmt.Errorf("Terraform HCL meta hook: %w", err)
	}

	return meta.generateConfig(cfginfos)
}

func (meta *baseMeta) providerConfig() string {
	if meta.devProvider {
		return fmt.Sprintf(`terraform {
  backend %q {}
}

provider "azurerm" {
  features {}
}
`, meta.backendType)
	}

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

func (meta baseMeta) filenameProviderSetting() string {
	if meta.useSafeFilename {
		return "provider.aztfy.tf"
	}
	return "provider.tf"
}

func (meta baseMeta) filenameMainCfg() string {
	if meta.useSafeFilename {
		return "main.aztfy.tf"
	}
	return "main.tf"
}

func (meta baseMeta) filenameTmpCfg() string {
	return "tmp.aztfy.tf"
}

func (meta *baseMeta) initProvider(ctx context.Context) error {
	// If the directory is empty, generate the full config as the output directory is empty.
	// Otherwise:
	// - If the output directory already exists the `azurerm` provider setting, then do nothing
	// - Otherwise, just generate the `azurerm` provider setting (as it is only for local backend)
	if meta.empty {
		cfgFile := filepath.Join(meta.outdir, meta.filenameProviderSetting())
		if err := utils.WriteFileSync(cfgFile, []byte(meta.providerConfig()), 0644); err != nil {
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

func (meta baseMeta) stateToConfig(ctx context.Context, list ImportList) (ConfigInfos, error) {
	out := ConfigInfos{}
	for _, item := range list.Imported() {
		b, err := tfadd.State(ctx, meta.tf, tfadd.Target(item.TFAddr.String()), tfadd.Full(meta.fullConfig))
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

func (meta baseMeta) terraformMetaHook(configs ConfigInfos, cfgTrans ...TFConfigTransformer) (ConfigInfos, error) {
	var err error
	for _, trans := range cfgTrans {
		configs, err = trans(configs)
		if err != nil {
			return nil, err
		}
	}
	return configs, nil
}

func (meta baseMeta) generateConfig(cfgs ConfigInfos) error {
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

func (meta baseMeta) cleanupTerraformAdd(tpl string) string {
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

// lifecycleAddon adds lifecycle meta arguments for some identified resources, which are mandatory to make them usable.
func (meta baseMeta) lifecycleAddon(configs ConfigInfos) (ConfigInfos, error) {
	out := make(ConfigInfos, len(configs))
	for i, cfg := range configs {
		switch cfg.TFAddr.Type {
		case "azurerm_application_insights_web_test":
			if err := hclBlockAppendLifecycle(cfg.hcl.Body().Blocks()[0].Body(), []string{"tags"}); err != nil {
				return nil, fmt.Errorf("azurerm_application_insights_web_test: %v", err)
			}
		}
		out[i] = cfg
	}
	return out, nil
}

func (meta baseMeta) addDependency(configs ConfigInfos) (ConfigInfos, error) {
	if err := configs.AddDependency(); err != nil {
		return nil, err
	}

	var out ConfigInfos

	configSet := map[string]ConfigInfo{}
	for _, cfg := range configs {
		configSet[cfg.AzureResourceID.String()] = cfg
	}

	for _, cfg := range configs {
		if len(cfg.DependsOn) != 0 {
			if err := hclBlockAppendDependency(cfg.hcl.Body().Blocks()[0].Body(), cfg.DependsOn, configSet); err != nil {
				return nil, err
			}
		}
		out = append(out, cfg)
	}

	return out, nil
}

func removeEverythingUnder(path string) error {
	// #nosec G304
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
	if err := dir.Close(); err != nil {
		return fmt.Errorf("closing dir %s: %v", path, err)
	}
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
	// #nosec G304
	dir, err := os.Open(path)
	if err != nil {
		return false, err
	}
	_, err = dir.Readdirnames(1)
	if err != nil {
		if err == io.EOF {
			if err := dir.Close(); err != nil {
				return false, fmt.Errorf("closing dir %s: %v", path, err)
			}
			return true, nil
		}
		return false, err
	}
	if err := dir.Close(); err != nil {
		return false, fmt.Errorf("closing dir %s: %v", path, err)
	}
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
	// #nosec G304
	dir, err := os.Open(path)
	if err != nil {
		return false, err
	}
	// #nosec G307
	defer dir.Close()

	fnames, err := dir.Readdirnames(0)
	if err != nil {
		return false, err
	}

	// Ideally, we shall use hclgrep for a perfect match. But as the provider setting is simple enough, we do a text matching here.
	p := regexp.MustCompile(`^\s*provider\s+"azurerm"\s*{\s*$`)
	for _, fname := range fnames {
		if filepath.Ext(fname) != ".tf" {
			continue
		}
		// #nosec G304
		f, err := os.Open(filepath.Join(path, fname))
		if err != nil {
			return false, fmt.Errorf("opening %s: %v", fname, err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if p.MatchString(scanner.Text()) {
				_ = f.Close()
				return true, nil
			}
		}
		if err := scanner.Err(); err != nil {
			_ = f.Close()
			return false, fmt.Errorf("reading file %s: %v", fname, err)
		}
		if err := f.Close(); err != nil {
			return false, fmt.Errorf("closing file %s: %v", fname, err)
		}
	}
	return false, nil
}

func appendToFile(path, content string) error {
	// #nosec G304
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	// #nosec G307
	defer f.Close()

	_, err = f.WriteString(content)
	return err
}

func resourceNamePattern(p string) (prefix, suffix string) {
	if pos := strings.LastIndex(p, "*"); pos != -1 {
		return p[:pos], p[pos+1:]
	}
	return p, ""
}

func ptr[T any](v T) *T {
	return &v
}
