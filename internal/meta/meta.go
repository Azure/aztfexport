package meta

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Azure/aztfy/internal/client"
	"github.com/Azure/aztfy/internal/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/magodo/tfadd/providers/azurerm"
	"github.com/magodo/tfadd/tfadd"
)

type TFConfigTransformer func(configs ConfigInfos) (ConfigInfos, error)

type meta interface {
	Init() error
	Workspace() string
	Import(item *ImportItem)
	CleanTFState(addr string)
	GenerateCfg(ImportList) error
}

var _ meta = &Meta{}

type Meta struct {
	subscriptionId string
	rootdir        string
	outdir         string
	tf             *tfexec.Terraform
	clientBuilder  *client.ClientBuilder
	backendType    string
	backendConfig  []string
	// Use a safer name which is less likely to conflicts with users' existing files.
	// This is mainly used for the --append option.
	useSafeFilename bool
}

func NewMeta(cfg config.CommonConfig) (*Meta, error) {
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
				fmt.Printf(`
The output directory is not empty. Please choose one of actions below:

* To overwrite everything inside the output directory, press "O"
* To append (state and config) into the output directory, press "A"
* Press other keys to quit
`)
				var ans string
				fmt.Scanf("%s", &ans)
				switch strings.ToLower(ans) {
				case "o":
					if err := removeEverythingUnder(outdir); err != nil {
						return nil, err
					}
				case "a":
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

	// AzureRM provider will honor env.var "AZURE_HTTP_USER_AGENT" when constructing for HTTP "User-Agent" header.
	os.Setenv("AZURE_HTTP_USER_AGENT", "aztfy")

	// Avoid the AzureRM provider to call the expensive RP listing API, repeatedly.
	os.Setenv("ARM_PROVIDER_ENHANCED_VALIDATION", "false")
	os.Setenv("ARM_SKIP_PROVIDER_REGISTRATION", "true")

	meta := &Meta{
		subscriptionId:  cfg.SubscriptionId,
		rootdir:         rootdir,
		outdir:          outdir,
		clientBuilder:   b,
		backendType:     cfg.BackendType,
		backendConfig:   cfg.BackendConfig,
		useSafeFilename: cfg.Append,
	}

	return meta, nil
}

func (meta Meta) Workspace() string {
	return meta.outdir
}

func (meta *Meta) Init() error {
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

func (meta *Meta) CleanTFState(addr string) {
	ctx := context.TODO()
	meta.tf.StateRm(ctx, addr)
}

func (meta Meta) Import(item *ImportItem) {
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

func (meta Meta) GenerateCfg(l ImportList) error {
	return meta.generateCfg(l, meta.lifecycleAddon)
}

func (meta Meta) generateCfg(l ImportList, cfgTrans ...TFConfigTransformer) error {
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

func (meta *Meta) providerConfig() string {
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

func (meta Meta) filenameProviderSetting() string {
	if meta.useSafeFilename {
		return "provider.aztfy.tf"
	}
	return "provider.tf"
}

func (meta Meta) filenameMainCfg() string {
	if meta.useSafeFilename {
		return "main.aztfy.tf"
	}
	return "main.tf"
}

func (meta Meta) filenameTmpCfg() string {
	return "tmp.aztfy.tf"
}

func (meta *Meta) initProvider(ctx context.Context) error {
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

func (meta Meta) stateToConfig(ctx context.Context, list ImportList) (ConfigInfos, error) {
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

func (meta Meta) terraformMetaHook(configs ConfigInfos, cfgTrans ...TFConfigTransformer) (ConfigInfos, error) {
	var err error
	for _, trans := range cfgTrans {
		configs, err = trans(configs)
		if err != nil {
			return nil, err
		}
	}
	return configs, nil
}

func (meta Meta) generateConfig(cfgs ConfigInfos) error {
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

func (meta Meta) cleanupTerraformAdd(tpl string) string {
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
			return false, fmt.Errorf("opening %s: %v", fname, err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if p.MatchString(scanner.Text()) {
				f.Close()
				return true, nil
			}
		}
		if err := scanner.Err(); err != nil {
			f.Close()
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
