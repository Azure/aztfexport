package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/Azure/aztfexport/pkg/config"
	"github.com/Azure/aztfexport/pkg/log"
	"github.com/zclconf/go-cty/cty"

	"github.com/Azure/aztfexport/internal/client"
	"github.com/Azure/aztfexport/internal/resmap"
	"github.com/Azure/aztfexport/internal/utils"
	"github.com/Azure/aztfexport/pkg/telemetry"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	tfclient "github.com/magodo/terraform-client-go/tfclient"
	"github.com/magodo/terraform-client-go/tfclient/configschema"
	"github.com/magodo/terraform-client-go/tfclient/typ"
	"github.com/magodo/tfadd/providers/azurerm"
	"github.com/magodo/tfadd/tfadd"
	"github.com/magodo/tfmerge/tfmerge"
	"github.com/magodo/tfstate"
	"github.com/magodo/workerpool"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

const ResourceMappingFileName = "aztfexportResourceMapping.json"
const SkippedResourcesFileName = "aztfexportSkippedResources.txt"

type TFConfigTransformer func(configs ConfigInfos) (ConfigInfos, error)

type BaseMeta interface {
	// Init initializes the base meta, including initialize terraform, provider and soem runtime temporary resources.
	Init(ctx context.Context) error
	// DeInit deinitializes the base meta, including cleaning up runtime temporary resources.
	DeInit(ctx context.Context) error
	// Workspace returns the path of the output directory.
	Workspace() string
	// ParallelImport imports the specified import list in parallel (parallelism is set during the meta builder function).
	// Import error won't be returned in the error, but is recorded in each ImportItem.
	ParallelImport(ctx context.Context, items []*ImportItem) error
	// PushState pushes the terraform state file (the base state of the workspace, adding the newly imported resources) back to the workspace.
	PushState(ctx context.Context) error
	// CleanTFState clean up the specified TF resource from the workspace's state file.
	CleanTFState(ctx context.Context, addr string)
	// GenerateCfg generates the TF configuration of the import list. Only resources successfully imported will be processed.
	GenerateCfg(ctx context.Context, l ImportList) error
	// ExportSkippedResources writes a file listing record resources that are skipped to be imported to the output directory.
	ExportSkippedResources(ctx context.Context, l ImportList) error
	// ExportResourceMapping writes a resource mapping file to the output directory.
	ExportResourceMapping(ctx context.Context, l ImportList) error
	// CleanUpWorkspace is a weired method that is only meant to be used internally by aztfexport, which under the hood will remove everything in the output directory, except the generated TF config.
	// This method does nothing if HCLOnly in the Config is not set.
	CleanUpWorkspace(ctx context.Context) error
}

var _ BaseMeta = &baseMeta{}

type baseMeta struct {
	subscriptionId    string
	azureSDKCred      azcore.TokenCredential
	azureSDKClientOpt arm.ClientOptions
	outdir            string
	outputFileNames   config.OutputFileNames
	tf                *tfexec.Terraform
	resourceClient    *armresources.Client
	providerVersion   string
	devProvider       bool
	backendType       string
	backendConfig     []string
	providerConfig    map[string]cty.Value
	fullConfig        bool
	parallelism       int

	hclOnly  bool
	tfclient tfclient.Client

	// The module address prefix in the resource addr. E.g. module.mod1.module.mod2.azurerm_resource_group.test.
	// This is an empty string if module path is not specified.
	moduleAddr string
	// The module directory in the fs where the terraform config should be generated to. This does not necessarily have the same structure as moduleAddr.
	// This is the same as the outdir if module path is not specified.
	moduleDir string

	// Parallel import supports
	importBaseDirs   []string
	importModuleDirs []string
	importTFs        []*tfexec.Terraform

	// The original base state, which is retrieved prior to the import, and is compared with the actual base state prior to the mutated state is pushed,
	// to ensure the base state has no out of band changes during the importing.
	originBaseState []byte
	// The current base state, which is mutated during the importing
	baseState []byte

	tc telemetry.Client
}

func NewBaseMeta(cfg config.CommonConfig) (*baseMeta, error) {
	if cfg.Parallelism == 0 {
		return nil, fmt.Errorf("Parallelism not set in the config")
	}
	if cfg.ProviderVersion != "" && cfg.DevProvider {
		return nil, fmt.Errorf("ProviderVersion conflicts with DevProvider in the config")
	}
	if cfg.TFClient != nil && !cfg.HCLOnly {
		return nil, fmt.Errorf("TFClient must be used together with HCLOnly")
	}

	// Determine the module directory and module address
	var (
		moduleAddr string
		moduleDir  = cfg.OutputDir
	)
	if cfg.ModulePath != "" {
		modulePaths := strings.Split(cfg.ModulePath, ".")

		// Resolve the Terraform module address
		var segs []string
		for _, moduleName := range modulePaths {
			segs = append(segs, "module."+moduleName)
		}
		moduleAddr = strings.Join(segs, ".")

		var err error
		moduleDir, err = getModuleDir(modulePaths, cfg.OutputDir)
		if err != nil {
			return nil, err
		}
	}

	// Construct Azure resources client
	b := client.ClientBuilder{
		Credential: cfg.AzureSDKCredential,
		Opt:        cfg.AzureSDKClientOption,
	}
	resClient, err := b.NewResourcesClient(cfg.SubscriptionId)
	if err != nil {
		return nil, fmt.Errorf("new resource client")
	}

	// Consider setting below environment variables via `tf.SetEnv()` once issue https://github.com/hashicorp/terraform-exec/issues/337 is resolved.

	// AzureRM provider will honor env.var "AZURE_HTTP_USER_AGENT" when constructing for HTTP "User-Agent" header.
	// #nosec G104
	os.Setenv("AZURE_HTTP_USER_AGENT", cfg.AzureSDKClientOption.Telemetry.ApplicationID)

	// Avoid the AzureRM provider to call the expensive RP listing API, repeatedly.
	// #nosec G104
	os.Setenv("ARM_PROVIDER_ENHANCED_VALIDATION", "false")
	// #nosec G104
	os.Setenv("ARM_SKIP_PROVIDER_REGISTRATION", "true")

	outputFileNames := cfg.OutputFileNames
	if outputFileNames.TerraformFileName == "" {
		outputFileNames.TerraformFileName = "terraform.tf"
	}
	if outputFileNames.ProviderFileName == "" {
		outputFileNames.ProviderFileName = "provider.tf"
	}
	if outputFileNames.MainFileName == "" {
		outputFileNames.MainFileName = "main.tf"
	}

	tc := cfg.TelemetryClient
	if tc == nil {
		tc = telemetry.NewNullClient()
	}

	if !cfg.DevProvider && cfg.ProviderVersion == "" {
		cfg.ProviderVersion = azurerm.ProviderSchemaInfo.Version
	}

	meta := &baseMeta{
		subscriptionId:    cfg.SubscriptionId,
		azureSDKCred:      cfg.AzureSDKCredential,
		azureSDKClientOpt: cfg.AzureSDKClientOption,
		outdir:            cfg.OutputDir,
		outputFileNames:   outputFileNames,
		resourceClient:    resClient,
		providerVersion:   cfg.ProviderVersion,
		devProvider:       cfg.DevProvider,
		backendType:       cfg.BackendType,
		backendConfig:     cfg.BackendConfig,
		providerConfig:    cfg.ProviderConfig,
		fullConfig:        cfg.FullConfig,
		parallelism:       cfg.Parallelism,
		hclOnly:           cfg.HCLOnly,
		tfclient:          cfg.TFClient,

		moduleAddr: moduleAddr,
		moduleDir:  moduleDir,

		tc: tc,
	}

	return meta, nil
}

func (meta baseMeta) Workspace() string {
	return meta.outdir
}

func (meta *baseMeta) Init(ctx context.Context) error {
	meta.tc.Trace(telemetry.Info, "Init Enter")
	defer meta.tc.Trace(telemetry.Info, "Init Leave")

	if meta.tfclient != nil {
		return meta.init_notf(ctx)
	}

	return meta.init_tf(ctx)
}

func (meta baseMeta) DeInit(ctx context.Context) error {
	meta.tc.Trace(telemetry.Info, "DeInit Enter")
	defer meta.tc.Trace(telemetry.Info, "DeInit Leave")

	if meta.tfclient != nil {
		return meta.deinit_notf(ctx)
	}

	return meta.deinit_tf(ctx)
}

func (meta *baseMeta) CleanTFState(ctx context.Context, addr string) {
	// Noop if tfclient is set
	if meta.tfclient != nil {
		return
	}

	// #nosec G104
	meta.tf.StateRm(ctx, addr)
}

func (meta *baseMeta) ParallelImport(ctx context.Context, items []*ImportItem) error {
	meta.tc.Trace(telemetry.Info, "ParallelImport Enter")
	defer meta.tc.Trace(telemetry.Info, "ParallelImport Leave")
	itemsCh := make(chan *ImportItem, len(items))
	for _, item := range items {
		itemsCh <- item
	}
	close(itemsCh)

	wp := workerpool.NewWorkPool(meta.parallelism)

	wp.Run(func(i interface{}) error {
		idx := i.(int)

		// Noop if tfclient is set
		if meta.tfclient != nil {
			return nil
		}

		stateFile := filepath.Join(meta.importBaseDirs[idx], "terraform.tfstate")

		// Don't merge state file if this import dir doesn't contain state file, which can because either this import dir imported nothing, or it encountered import error
		if _, err := os.Stat(stateFile); os.IsNotExist(err) {
			return nil
		}
		// Ensure the state file is removed after this round import, preparing for the next round.
		defer os.Remove(stateFile)

		log.Printf("[DEBUG] Merging terraform state file %s (tfmerge)", stateFile)
		newState, err := tfmerge.Merge(ctx, meta.tf, meta.baseState, stateFile)
		if err != nil {
			return fmt.Errorf("failed to merge state file: %v", err)
		}
		meta.baseState = newState

		return nil
	})

	for i := 0; i < meta.parallelism; i++ {
		i := i
		wp.AddTask(func() (interface{}, error) {
			for item := range itemsCh {
				meta.importItem(ctx, item, i)
			}
			return i, nil
		})
	}

	// #nosec G104
	if err := wp.Done(); err != nil {
		return err
	}

	return nil
}

func (meta baseMeta) PushState(ctx context.Context) error {
	meta.tc.Trace(telemetry.Info, "PushState Enter")
	defer meta.tc.Trace(telemetry.Info, "PushState Leave")

	// Noop if tfclient is set
	if meta.tfclient != nil {
		return nil
	}

	// Don't push state if there is no state to push. This might happen when all the resources failed to import with "--continue".
	if len(meta.baseState) == 0 {
		return nil
	}

	// Ensure there is no out of band change on the base state
	baseState, err := meta.tf.StatePull(ctx)
	if err != nil {
		return fmt.Errorf("failed to pull state: %v", err)
	}
	if baseState != string(meta.originBaseState) {
		edits := myers.ComputeEdits(span.URIFromPath("origin.tfstate"), string(meta.originBaseState), baseState)
		changes := fmt.Sprint(gotextdiff.ToUnified("origin.tfstate", "current.tfstate", string(meta.originBaseState), edits))
		return fmt.Errorf("there is out-of-band changes on the state file:\n%s", changes)
	}

	// Create a temporary state file to hold the merged states, then push the state to the output directory.
	f, err := os.CreateTemp("", "")
	if err != nil {
		return fmt.Errorf("creating a temporary state file: %v", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing the temporary state file %s: %v", f.Name(), err)
	}
	// #nosec G306
	if err := os.WriteFile(f.Name(), meta.baseState, 0644); err != nil {
		return fmt.Errorf("writing to the temporary state file: %v", err)
	}

	defer os.Remove(f.Name())

	if err := meta.tf.StatePush(ctx, f.Name(), tfexec.Lock(true)); err != nil {
		return fmt.Errorf("failed to push state: %v", err)
	}

	return nil
}

func (meta baseMeta) GenerateCfg(ctx context.Context, l ImportList) error {
	meta.tc.Trace(telemetry.Info, "GenerateCfg Enter")
	defer meta.tc.Trace(telemetry.Info, "GenerateCfg Leave")
	return meta.generateCfg(ctx, l, meta.lifecycleAddon, meta.addDependency)
}

func (meta baseMeta) ExportResourceMapping(_ context.Context, l ImportList) error {
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
	output := filepath.Join(meta.outdir, ResourceMappingFileName)
	b, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return fmt.Errorf("JSON marshalling the resource mapping: %v", err)
	}
	// #nosec G306
	if err := os.WriteFile(output, b, 0644); err != nil {
		return fmt.Errorf("writing the resource mapping to %s: %v", output, err)
	}
	return nil
}

func (meta baseMeta) ExportSkippedResources(_ context.Context, l ImportList) error {
	var sl []string
	for _, item := range l {
		if item.Skip() {
			sl = append(sl, "- "+item.AzureResourceID.String())
		}
	}
	if len(sl) == 0 {
		return nil
	}

	output := filepath.Join(meta.outdir, SkippedResourcesFileName)
	// #nosec G306
	if err := os.WriteFile(output, []byte(fmt.Sprintf(`Following resources are marked to be skipped:

%s
`, strings.Join(sl, "\n"))), 0644); err != nil {
		return fmt.Errorf("writing the skipped resources to %s: %v", output, err)
	}
	return nil
}

func (meta baseMeta) CleanUpWorkspace(_ context.Context) error {
	// For hcl only mode with using terraform binary, we will have to clean up everything under the output directory,
	// except for the TF code, resource mapping file and ignore list file.
	if meta.hclOnly && meta.tfclient == nil {
		tmpDir, err := os.MkdirTemp("", "")
		if err != nil {
			return err
		}
		defer func() {
			// #nosec G104
			os.RemoveAll(tmpDir)
		}()

		tmpMainCfg := filepath.Join(tmpDir, meta.outputFileNames.MainFileName)
		tmpProviderCfg := filepath.Join(tmpDir, meta.outputFileNames.ProviderFileName)
		tmpResourceMappingFileName := filepath.Join(tmpDir, ResourceMappingFileName)
		tmpSkippedResourcesFileName := filepath.Join(tmpDir, SkippedResourcesFileName)

		if err := utils.CopyFile(filepath.Join(meta.outdir, meta.outputFileNames.MainFileName), tmpMainCfg); err != nil {
			return err
		}
		if err := utils.CopyFile(filepath.Join(meta.outdir, meta.outputFileNames.ProviderFileName), tmpProviderCfg); err != nil {
			return err
		}
		if err := utils.CopyFile(filepath.Join(meta.outdir, ResourceMappingFileName), tmpResourceMappingFileName); err != nil {
			return err
		}
		if err := utils.CopyFile(filepath.Join(meta.outdir, SkippedResourcesFileName), tmpSkippedResourcesFileName); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}

		if err := utils.RemoveEverythingUnder(meta.outdir); err != nil {
			return err
		}

		if err := utils.CopyFile(tmpMainCfg, filepath.Join(meta.outdir, meta.outputFileNames.MainFileName)); err != nil {
			return err
		}
		if err := utils.CopyFile(tmpProviderCfg, filepath.Join(meta.outdir, meta.outputFileNames.ProviderFileName)); err != nil {
			return err
		}
		if err := utils.CopyFile(tmpResourceMappingFileName, filepath.Join(meta.outdir, ResourceMappingFileName)); err != nil {
			return err
		}
		if err := utils.CopyFile(tmpSkippedResourcesFileName, filepath.Join(meta.outdir, SkippedResourcesFileName)); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}

	return nil
}

func (meta baseMeta) generateCfg(ctx context.Context, l ImportList, cfgTrans ...TFConfigTransformer) error {
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

func (meta *baseMeta) buildTerraformConfigForImportDir() string {
	if meta.devProvider {
		return "terraform {}"
	}

	return fmt.Sprintf(`terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
      version = "%s"
    }
  }
}
`, meta.providerVersion)
}

func (meta *baseMeta) buildTerraformConfig(backendType string) string {
	if meta.devProvider {
		return fmt.Sprintf(`terraform {
  backend %q {}
}
`, backendType)
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
`, backendType, meta.providerVersion)
}

func (meta *baseMeta) buildProviderConfig() string {
	f := hclwrite.NewEmptyFile()
	body := f.Body().AppendNewBlock("provider", []string{"azurerm"}).Body()
	body.AppendNewBlock("features", nil)
	for k, v := range meta.providerConfig {
		body.SetAttributeValue(k, v)
	}
	return string(f.Bytes())
}

func (meta *baseMeta) init_notf(ctx context.Context) error {
	schResp, diags := meta.tfclient.GetProviderSchema()
	if diags.HasErrors() {
		return fmt.Errorf("getting provider schema: %v", diags)
	}

	// Ensure "features" is always defined in the provider config
	providerConfig := map[string]cty.Value{
		"features": cty.ListValEmpty(configschema.SchemaBlockImpliedType(schResp.Provider.Block.NestedBlocks["features"].Block)),
	}
	for k, v := range meta.providerConfig {
		providerConfig[k] = v
	}
	b, err := json.Marshal(meta.providerConfig)
	if err != nil {
		return fmt.Errorf("marshal provider config: %v", err)
	}
	config, err := ctyjson.Unmarshal(b, configschema.SchemaBlockImpliedType(schResp.Provider.Block))
	if err != nil {
		return fmt.Errorf("unmarshal provider config: %v", err)
	}

	if _, diags = meta.tfclient.ConfigureProvider(ctx, typ.ConfigureProviderRequest{
		Config: config,
	}); diags.HasErrors() {
		return fmt.Errorf("configure provider: %v", diags)
	}

	return nil
}

func (meta *baseMeta) init_tf(ctx context.Context) error {
	// Create the import directories per parallelism
	if err := meta.initImportDirs(); err != nil {
		return err
	}

	// Init terraform
	if err := meta.initTF(ctx); err != nil {
		return err
	}

	// Init provider
	if err := meta.initProvider(ctx); err != nil {
		return err
	}

	// Pull TF state
	baseState, err := meta.tf.StatePull(ctx)
	if err != nil {
		return fmt.Errorf("failed to pull state: %v", err)
	}
	meta.baseState = []byte(baseState)
	meta.originBaseState = []byte(baseState)

	return nil
}

func (meta *baseMeta) initImportDirs() error {
	var importBaseDirs []string
	var importModuleDirs []string
	modulePaths := []string{}
	for i, v := range strings.Split(meta.moduleAddr, ".") {
		if i%2 == 1 {
			modulePaths = append(modulePaths, v)
		}
	}
	for i := 0; i < meta.parallelism; i++ {
		dir, err := os.MkdirTemp("", "aztfexport-")
		if err != nil {
			return fmt.Errorf("creating import directory: %v", err)
		}

		// Creating the module hierarchy if module path is specified.
		// The hierarchy used here is not necessarily to be the same as is defined. What we need to guarantee here is the module address in TF is as specified.
		mdir := dir
		for _, moduleName := range modulePaths {
			fpath := filepath.Join(mdir, "main.tf")
			// #nosec G306
			if err := os.WriteFile(fpath, []byte(fmt.Sprintf(`module "%s" {
  source = "./%s"
}
`, moduleName, moduleName)), 0644); err != nil {
				return fmt.Errorf("creating %s: %v", fpath, err)
			}

			mdir = filepath.Join(mdir, moduleName)
			// #nosec G301
			if err := os.Mkdir(mdir, 0750); err != nil {
				return fmt.Errorf("creating module dir %s: %v", mdir, err)
			}
		}

		importModuleDirs = append(importModuleDirs, mdir)
		importBaseDirs = append(importBaseDirs, dir)
	}
	meta.importBaseDirs = importBaseDirs
	meta.importModuleDirs = importModuleDirs
	return nil
}

func (meta *baseMeta) initTF(ctx context.Context) error {
	log.Printf("[INFO] Init Terraform")
	execPath, err := FindTerraform(ctx)
	if err != nil {
		return fmt.Errorf("error finding a terraform exectuable: %w", err)
	}
	log.Printf("[INFO] Find terraform binary at %s", execPath)

	newTF := func(dir string) (*tfexec.Terraform, error) {
		tf, err := tfexec.NewTerraform(dir, execPath)
		if err != nil {
			return nil, fmt.Errorf("error running NewTerraform: %w", err)
		}
		if v, ok := os.LookupEnv("TF_LOG_PATH"); ok {
			// #nosec G104
			tf.SetLogPath(v)
		}
		if v, ok := os.LookupEnv("TF_LOG"); ok {
			// #nosec G104
			tf.SetLog(v)
		}
		return tf, nil
	}

	tf, err := newTF(meta.outdir)
	if err != nil {
		return fmt.Errorf("failed to init terraform: %w", err)
	}
	meta.tf = tf

	for _, importDir := range meta.importBaseDirs {
		tf, err := newTF(importDir)
		if err != nil {
			return fmt.Errorf("failed to init terraform: %w", err)
		}
		meta.importTFs = append(meta.importTFs, tf)
	}

	return nil
}

func (meta *baseMeta) initProvider(ctx context.Context) error {
	log.Printf("[INFO] Init provider")

	module, diags := tfconfig.LoadModule(meta.outdir)
	if diags.HasErrors() {
		return diags.Err()
	}

	tfblock, err := utils.InspecTerraformBlock(meta.outdir)
	if err != nil {
		return err
	}

	if module.ProviderConfigs["azurerm"] == nil {
		log.Printf("[INFO] Output directory doesn't contain provider setting, create one then")
		cfgFile := filepath.Join(meta.outdir, meta.outputFileNames.ProviderFileName)
		// #nosec G306
		if err := os.WriteFile(cfgFile, []byte(meta.buildProviderConfig()), 0644); err != nil {
			return fmt.Errorf("error creating provider config: %w", err)
		}
	}

	if tfblock == nil {
		log.Printf("[INFO] Output directory doesn't contain terraform block, create one then")
		cfgFile := filepath.Join(meta.outdir, meta.outputFileNames.TerraformFileName)
		// #nosec G306
		if err := os.WriteFile(cfgFile, []byte(meta.buildTerraformConfig(meta.backendType)), 0644); err != nil {
			return fmt.Errorf("error creating terraform config: %w", err)
		}
	}

	// Initialize provider for the output directory.
	var opts []tfexec.InitOption
	for _, opt := range meta.backendConfig {
		opts = append(opts, tfexec.BackendConfig(opt))
	}

	log.Printf(`[DEBUG] Run "terraform init" for the output directory %s`, meta.outdir)
	if err := meta.tf.Init(ctx, opts...); err != nil {
		return fmt.Errorf("error running terraform init for the output directory: %s", err)
	}

	// Initialize provider for the import directories.
	wp := workerpool.NewWorkPool(meta.parallelism)
	wp.Run(nil)
	for i := range meta.importBaseDirs {
		i := i
		wp.AddTask(func() (interface{}, error) {
			providerFile := filepath.Join(meta.importBaseDirs[i], "provider.tf")
			// #nosec G306
			if err := os.WriteFile(providerFile, []byte(meta.buildProviderConfig()), 0644); err != nil {
				return nil, fmt.Errorf("error creating provider config: %w", err)
			}
			terraformFile := filepath.Join(meta.importBaseDirs[i], "terraform.tf")
			// #nosec G306
			if err := os.WriteFile(terraformFile, []byte(meta.buildTerraformConfigForImportDir()), 0644); err != nil {
				return nil, fmt.Errorf("error creating terraform config: %w", err)
			}
			if meta.devProvider {
				log.Printf(`[DEBUG] Skip running "terraform init" for the import directory (dev provider): %s`, meta.importBaseDirs[i])
			} else {
				log.Printf(`[DEBUG] Run "terraform init" for the import directory %s`, meta.importBaseDirs[i])
				if err := meta.importTFs[i].Init(ctx); err != nil {
					return nil, fmt.Errorf("error running terraform init: %s", err)
				}
			}
			return nil, nil
		})
	}
	if err := wp.Done(); err != nil {
		return fmt.Errorf("initializing provider for the import directories: %v", err)
	}

	return nil
}

func (meta *baseMeta) importItem(ctx context.Context, item *ImportItem, importIdx int) {
	if item.Skip() {
		log.Printf("[INFO] Skipping %s", item.TFResourceId)
		return
	}

	if meta.tfclient != nil {
		meta.importItem_notf(ctx, item, importIdx)
		return
	}

	meta.importItem_tf(ctx, item, importIdx)
}

func (meta *baseMeta) importItem_tf(ctx context.Context, item *ImportItem, importIdx int) {
	moduleDir := meta.importModuleDirs[importIdx]
	tf := meta.importTFs[importIdx]

	// Construct the empty cfg file for importing
	cfgFile := filepath.Join(moduleDir, "tmp.aztfexport.tf")
	tpl := fmt.Sprintf(`resource "%s" "%s" {}`, item.TFAddr.Type, item.TFAddr.Name)
	// #nosec G306
	if err := os.WriteFile(cfgFile, []byte(tpl), 0644); err != nil {
		err := fmt.Errorf("generating resource template file for %s: %w", item.TFAddr, err)
		log.Printf("[ERROR] %v", err)
		item.ImportError = err
		return
	}
	defer os.Remove(cfgFile)

	// Import resources
	addr := item.TFAddr.String()
	if meta.moduleAddr != "" {
		addr = meta.moduleAddr + "." + addr
	}

	log.Printf("[INFO] Importing %s as %s", item.TFResourceId, addr)
	// The actual resource type names in telemetry is redacted
	meta.tc.Trace(telemetry.Info, fmt.Sprintf("Importing %s as %s", item.AzureResourceID.TypeString(), addr))

	err := tf.Import(ctx, addr, item.TFResourceId)
	if err != nil {
		log.Printf("[ERROR] Importing %s: %v", item.TFAddr, err)
		meta.tc.Trace(telemetry.Error, fmt.Sprintf("Importing %s: %v", item.AzureResourceID.TypeString(), err))
	} else {
		meta.tc.Trace(telemetry.Info, fmt.Sprintf("Importing %s as %s successfully", item.AzureResourceID.TypeString(), addr))
	}
	item.ImportError = err
	item.Imported = err == nil
}

func (meta *baseMeta) importItem_notf(ctx context.Context, item *ImportItem, importIdx int) {
	// Import resources
	addr := item.TFAddr.String()
	log.Printf("[INFO] Importing %s as %s", item.TFResourceId, addr)
	// The actual resource type names in telemetry is redacted
	meta.tc.Trace(telemetry.Info, fmt.Sprintf("Importing %s as %s", item.AzureResourceID.TypeString(), addr))

	importResp, diags := meta.tfclient.ImportResourceState(ctx, typ.ImportResourceStateRequest{
		TypeName: item.TFAddr.Type,
		ID:       item.TFResourceId,
	})
	if diags.HasErrors() {
		log.Printf("[ERROR] Importing %s: %v", item.TFAddr, diags)
		meta.tc.Trace(telemetry.Error, fmt.Sprintf("Importing %s: %v", item.AzureResourceID.TypeString(), diags))
		item.ImportError = diags.Err()
		item.Imported = false
		return
	}
	if len(importResp.ImportedResources) != 1 {
		err := fmt.Errorf("expect 1 resource being imported, got=%d", len(importResp.ImportedResources))
		log.Printf("[ERROR] %s", err)
		meta.tc.Trace(telemetry.Error, err.Error())
		item.ImportError = err
		item.Imported = false
		return
	}
	res := importResp.ImportedResources[0]
	readResp, diags := meta.tfclient.ReadResource(ctx, typ.ReadResourceRequest{
		TypeName:   res.TypeName,
		PriorState: res.State,
		Private:    res.Private,
	})
	if diags.HasErrors() {
		log.Printf("[ERROR] Reading %s: %v", item.TFAddr, diags)
		meta.tc.Trace(telemetry.Error, fmt.Sprintf("Reading %s: %v", item.AzureResourceID.TypeString(), diags))
		item.ImportError = diags.Err()
		item.Imported = false
		return
	}

	item.State = readResp.NewState
	item.ImportError = nil
	item.Imported = true
	return
}

func (meta baseMeta) stateToConfig(ctx context.Context, list ImportList) (ConfigInfos, error) {
	var out []ConfigInfo
	var bs [][]byte

	importedList := list.Imported()

	if meta.tfclient != nil {
		for _, item := range importedList {
			schResp, diags := meta.tfclient.GetProviderSchema()
			if diags.HasErrors() {
				return nil, fmt.Errorf("get provider schema: %v", diags)
			}
			rsch, ok := schResp.ResourceTypes[item.TFAddr.Type]
			if !ok {
				return nil, fmt.Errorf("no resource schema for %s found in the provider schema", item.TFAddr.Type)
			}
			b, err := tfadd.GenerateForOneResource(
				&rsch,
				tfstate.StateResource{
					Mode:         tfjson.ManagedResourceMode,
					Address:      item.TFAddr.String(),
					Type:         item.TFAddr.Type,
					ProviderName: "registry.terraform.io/hashicorp/azurerm",
					Value:        item.State,
				},
				meta.fullConfig)
			if err != nil {
				return nil, fmt.Errorf("generating state for resource %s: %v", item.TFAddr, err)
			}
			bs = append(bs, b)
		}
	} else {
		var addrs []string
		for _, item := range importedList {
			addr := item.TFAddr.String()
			if meta.moduleAddr != "" {
				addr = meta.moduleAddr + "." + addr
			}
			addrs = append(addrs, addr)
		}

		var err error
		bs, err = tfadd.StateForTargets(ctx, meta.tf, addrs, tfadd.Full(meta.fullConfig))
		if err != nil {
			return nil, fmt.Errorf("converting terraform state to config: %w", err)
		}
	}

	for i, b := range bs {
		tpl := meta.cleanupTerraformAdd(string(b))
		f, diag := hclwrite.ParseConfig([]byte(tpl), "", hcl.InitialPos)
		if diag.HasErrors() {
			return nil, fmt.Errorf("parsing the HCL generated by \"terraform add\" of %s: %s", importedList[i].TFAddr, diag.Error())
		}
		out = append(out, ConfigInfo{
			ImportItem: importedList[i],
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
	cfgFile := filepath.Join(meta.moduleDir, meta.outputFileNames.MainFileName)
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

func (meta *baseMeta) deinit_notf(ctx context.Context) error {
	meta.tfclient.Close()
	return nil
}

func (meta *baseMeta) deinit_tf(ctx context.Context) error {
	// Clean up the temporary workspaces for parallel import
	for _, dir := range meta.importBaseDirs {
		// #nosec G104
		os.RemoveAll(dir)
	}
	return nil
}

func getModuleDir(modulePaths []string, moduleDir string) (string, error) {
	// Ensure the module path is something called by the main module
	// We are following the module source and recursively call the LoadModule below. This is valid since we only support local path modules.
	// (remote sources are not supported since we will end up generating config to that module, it only makes sense for local path modules)
	module, err := tfconfig.LoadModule(moduleDir)
	if err != nil {
		return "", fmt.Errorf("loading main module: %v", err)
	}

	for i, moduleName := range modulePaths {
		mc := module.ModuleCalls[moduleName]
		if mc == nil {
			return "", fmt.Errorf("no module %q invoked by the root module", strings.Join(modulePaths[:i+1], "."))
		}
		// See https://developer.hashicorp.com/terraform/language/modules/sources#local-paths
		if !strings.HasPrefix(mc.Source, "./") && !strings.HasPrefix(mc.Source, "../") {
			return "", fmt.Errorf("the source of module %q is not a local path", strings.Join(modulePaths[:i+1], "."))
		}
		moduleDir = filepath.Join(moduleDir, mc.Source)
		module, err = tfconfig.LoadModule(moduleDir)
		if err != nil {
			return "", fmt.Errorf("loading module %q: %v", strings.Join(modulePaths[:i+1], "."), err)
		}
	}
	return moduleDir, nil
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
