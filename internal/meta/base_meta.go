package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/Azure/aztfexport/pkg/config"
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
	"github.com/magodo/tfadd/providers/azapi"
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
	// Logger returns a slog.Logger
	Logger() *slog.Logger
	// ProviderName returns the target provider name, which is either azurerm or azapi.
	ProviderName() string
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

	SetPreImportHook(config.ImportCallback)
	SetPostImportHook(config.ImportCallback)
}

var _ BaseMeta = &baseMeta{}

type baseMeta struct {
	logger            *slog.Logger
	subscriptionId    string
	azureSDKCred      azcore.TokenCredential
	azureSDKClientOpt arm.ClientOptions
	outdir            string
	outputFileNames   config.OutputFileNames
	tf                *tfexec.Terraform
	resourceClient    *armresources.Client
	providerVersion   string
	devProvider       bool
	providerName      string
	backendType       string
	backendConfig     []string
	providerConfig    map[string]cty.Value

	// tfadd options
	fullConfig    bool
	maskSensitive bool

	parallelism        int
	preImportHook      config.ImportCallback
	postImportHook     config.ImportCallback
	generateImportFile bool

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
	if outputFileNames.ImportBlockFileName == "" {
		outputFileNames.ImportBlockFileName = "import.tf"
	}

	tc := cfg.TelemetryClient
	if tc == nil {
		tc = telemetry.NewNullClient()
	}

	if !cfg.DevProvider && cfg.ProviderVersion == "" {
		switch cfg.ProviderName {
		case "azurerm":
			cfg.ProviderVersion = azurerm.ProviderSchemaInfo.Version
		case "azapi":
			cfg.ProviderVersion = azapi.ProviderSchemaInfo.Version
		}
	}

	// Update provider config if not explicitly defined
	providerConfig := cfg.ProviderConfig
	if providerConfig == nil {
		providerConfig = map[string]cty.Value{}
	}

	setIfNoExist := func(k string, v cty.Value) {
		if _, ok := providerConfig[k]; !ok {
			providerConfig[k] = v
		}
	}

	// Update provider config for auth config
	if cfg.SubscriptionId != "" {
		setIfNoExist("subscription_id", cty.StringVal(cfg.SubscriptionId))
	}
	if v := cfg.AuthConfig.Environment; v != "" {
		setIfNoExist("environment", cty.StringVal(v))
	}
	if v := cfg.AuthConfig.TenantID; v != "" {
		setIfNoExist("tenant_id", cty.StringVal(v))
	}

	if len(cfg.AuthConfig.AuxiliaryTenantIDs) != 0 {
		var tenantIds []cty.Value
		for _, id := range cfg.AuthConfig.AuxiliaryTenantIDs {
			tenantIds = append(tenantIds, cty.StringVal(id))
		}
		setIfNoExist("auxiliary_tenant_ids", cty.ListVal(tenantIds))
	}

	if v := cfg.AuthConfig.ClientID; v != "" {
		setIfNoExist("client_id", cty.StringVal(v))
	}
	if v := cfg.AuthConfig.ClientSecret; v != "" {
		setIfNoExist("client_secret", cty.StringVal(v))
	}
	if v := cfg.AuthConfig.ClientCertificateEncoded; v != "" {
		setIfNoExist("client_certificate", cty.StringVal(v))
	}
	if v := cfg.AuthConfig.ClientCertificatePassword; v != "" {
		setIfNoExist("client_certificate_password", cty.StringVal(v))
	}
	if v := cfg.AuthConfig.OIDCTokenRequestToken; v != "" {
		setIfNoExist("oidc_request_token", cty.StringVal(v))
	}
	if v := cfg.AuthConfig.OIDCTokenRequestURL; v != "" {
		setIfNoExist("oidc_request_url", cty.StringVal(v))
	}
	if v := cfg.AuthConfig.OIDCAssertionToken; v != "" {
		setIfNoExist("oidc_token", cty.StringVal(v))
	}
	setIfNoExist("use_msi", cty.BoolVal(cfg.AuthConfig.UseManagedIdentity))
	setIfNoExist("use_cli", cty.BoolVal(cfg.AuthConfig.UseAzureCLI))
	setIfNoExist("use_oidc", cty.BoolVal(cfg.AuthConfig.UseOIDC))

	// Update provider config for provider registration
	switch cfg.ProviderName {
	case "azurerm":
		setIfNoExist("resource_provider_registrations", cty.StringVal("none"))
	case "azapi":
		setIfNoExist("skip_provider_registration", cty.BoolVal(true))
	}

	meta := &baseMeta{
		logger:             cfg.Logger,
		subscriptionId:     cfg.SubscriptionId,
		azureSDKCred:       cfg.AzureSDKCredential,
		azureSDKClientOpt:  cfg.AzureSDKClientOption,
		outdir:             cfg.OutputDir,
		outputFileNames:    outputFileNames,
		resourceClient:     resClient,
		providerVersion:    cfg.ProviderVersion,
		devProvider:        cfg.DevProvider,
		backendType:        cfg.BackendType,
		backendConfig:      cfg.BackendConfig,
		providerConfig:     providerConfig,
		providerName:       cfg.ProviderName,
		fullConfig:         cfg.FullConfig,
		maskSensitive:      cfg.MaskSensitive,
		parallelism:        cfg.Parallelism,
		preImportHook:      cfg.PreImportHook,
		postImportHook:     cfg.PostImportHook,
		generateImportFile: cfg.GenerateImportBlock,
		hclOnly:            cfg.HCLOnly,
		tfclient:           cfg.TFClient,

		moduleAddr: moduleAddr,
		moduleDir:  moduleDir,

		tc: tc,
	}

	return meta, nil
}

func (meta baseMeta) Logger() *slog.Logger {
	return meta.logger
}

func (meta baseMeta) ProviderName() string {
	return meta.providerName
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

	total := len(items)
	itemsCh := make(chan *ImportItem, total)
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

		meta.Logger().Debug("Merging terraform state file (tfmerge)", "file", stateFile)
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
				iitem := config.ImportItem{
					AzureResourceID: item.AzureResourceID,
					TFResourceId:    item.TFResourceId,
					ImportError:     item.ImportError,
					TFAddr:          item.TFAddr,
				}
				startTime := time.Now()
				if meta.preImportHook != nil {
					meta.preImportHook(startTime, iitem)
				}
				meta.importItem(ctx, item, i)
				if meta.postImportHook != nil {
					meta.postImportHook(startTime, iitem)
				}
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

func (meta baseMeta) ExportResourceMapping(ctx context.Context, l ImportList) error {
	m := resmap.ResourceMapping{}
	for _, item := range l {
		if item.Skip() {
			continue
		}

		// The JSON mapping record
		m[item.AzureResourceID.String()] = resmap.ResourceMapEntity{
			ResourceId:   item.TFResourceId,
			ResourceType: item.TFAddr.Type,
			ResourceName: item.TFAddr.Name,
		}
	}
	b, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return fmt.Errorf("JSON marshalling the resource mapping: %v", err)
	}
	oMapFile := filepath.Join(meta.outdir, ResourceMappingFileName)
	// #nosec G306
	if err := os.WriteFile(oMapFile, b, 0644); err != nil {
		return fmt.Errorf("writing the resource mapping to %s: %v", oMapFile, err)
	}

	if meta.generateImportFile {
		f := hclwrite.NewFile()
		body := f.Body()
		for _, item := range l {
			if item.Skip() {
				continue
			}

			// The import block
			blk := hclwrite.NewBlock("import", nil)
			blk.Body().SetAttributeValue("id", cty.StringVal(item.TFResourceId))
			blk.Body().SetAttributeTraversal("to", hcl.Traversal{hcl.TraverseRoot{Name: item.TFAddr.Type}, hcl.TraverseAttr{Name: item.TFAddr.Name}})
			body.AppendBlock(blk)
		}
		oImportFile := filepath.Join(meta.moduleDir, meta.outputFileNames.ImportBlockFileName)
		// #nosec G306
		if err := os.WriteFile(oImportFile, f.Bytes(), 0644); err != nil {
			return fmt.Errorf("writing the import block to %s: %v", oImportFile, err)
		}
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
	// For hcl only mode with using terraform binary, we will have to clean up the state and terraform cli/provider related files the output directory,
	// except for the TF code, resource mapping file and ignore list file.
	if meta.hclOnly && meta.tfclient == nil {
		for _, entryName := range []string{
			"terraform.tfstate",
			".terraform",
			".terraform.lock.hcl",
		} {
			if err := os.RemoveAll(filepath.Join(meta.outdir, entryName)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (meta *baseMeta) SetPreImportHook(cb config.ImportCallback) {
	meta.preImportHook = cb
}

func (meta *baseMeta) SetPostImportHook(cb config.ImportCallback) {
	meta.postImportHook = cb
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

func (meta *baseMeta) useAzAPI() bool {
	return meta.providerName == "azapi"
}

func (meta *baseMeta) buildTerraformConfig(backendType string) string {
	backendLine := ""
	if backendType != "" {
		backendLine = "\n  backend \"" + backendType + "\" {}\n"
	}

	providerName := meta.providerName

	providerSource := "hashicorp/azurerm"
	if meta.useAzAPI() {
		providerSource = "azure/azapi"
	}

	providerVersionLine := ""
	if meta.providerVersion != "" {
		providerVersionLine = "\n      version = \"" + meta.providerVersion + "\"\n"
	}

	return fmt.Sprintf(`terraform {%s
  required_providers {
    %s = {
      source = %q%s
    }
  }
}
`, backendLine, providerName, providerSource, providerVersionLine)
}

func (meta *baseMeta) buildProviderConfig() string {
	f := hclwrite.NewEmptyFile()

	var body *hclwrite.Body
	if meta.useAzAPI() {
		body = f.Body().AppendNewBlock("provider", []string{"azapi"}).Body()
	} else {
		body = f.Body().AppendNewBlock("provider", []string{"azurerm"}).Body()
		body.AppendNewBlock("features", nil)
	}
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

	providerCfg := "{}"
	if !meta.useAzAPI() {
		// Ensure "features" is always defined in the azurerm provider initConfig
		providerCfg = `{"features": []}`
	}
	initConfig, err := ctyjson.Unmarshal([]byte(providerCfg), configschema.SchemaBlockImpliedType(schResp.Provider.Block))
	if err != nil {
		return fmt.Errorf("ctyjson unmarshal initial provider config: %v", err)
	}

	providerConfig := initConfig.AsValueMap()

	for k, v := range meta.providerConfig {
		providerConfig[k] = v
	}

	if _, diags = meta.tfclient.ConfigureProvider(ctx, typ.ConfigureProviderRequest{
		Config: cty.ObjectVal(providerConfig),
	}); diags.HasErrors() {
		return fmt.Errorf("configure provider: %v", diags)
	}

	return nil
}

func (meta *baseMeta) init_tf(ctx context.Context) error {
	// Consider setting below environment variables via `tf.SetEnv()` once issue https://github.com/hashicorp/terraform-exec/issues/337 is resolved.

	// Disable AzureRM provider's enahnced validation, which will cause RP listing, that is expensive.
	// The setting for notf version is done during the tfclient initialization in main function.
	// #nosec G104
	os.Setenv("ARM_PROVIDER_ENHANCED_VALIDATION", "false")

	// AzureRM provider will honor env.var "AZURE_HTTP_USER_AGENT" when constructing for HTTP "User-Agent" header.
	// The setting for notf version is done during the tfclient initialization in main function.
	// #nosec G104
	os.Setenv("AZURE_HTTP_USER_AGENT", meta.azureSDKClientOpt.Telemetry.ApplicationID)

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
	meta.Logger().Info("Init Terraform")
	execPath, err := FindTerraform(ctx)
	if err != nil {
		return fmt.Errorf("error finding a terraform exectuable: %w", err)
	}
	meta.Logger().Info("Found terraform binary", "path", execPath)

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
	meta.Logger().Info("Init provider")

	module, diags := tfconfig.LoadModule(meta.outdir)
	if diags.HasErrors() {
		return diags.Err()
	}

	tfblock, err := utils.InspecTerraformBlock(meta.outdir)
	if err != nil {
		return err
	}

	if module.ProviderConfigs[meta.providerName] == nil {
		meta.Logger().Info("Output directory doesn't contain provider setting, create one then")
		cfgFile := filepath.Join(meta.outdir, meta.outputFileNames.ProviderFileName)
		// #nosec G306
		if err := os.WriteFile(cfgFile, []byte(meta.buildProviderConfig()), 0644); err != nil {
			return fmt.Errorf("error creating provider config: %w", err)
		}
	}

	if tfblock == nil {
		meta.Logger().Info("Output directory doesn't contain terraform block, create one then")
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

	meta.Logger().Debug(`Run "terraform init" for the output directory`, "dir", meta.outdir)
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
			if err := os.WriteFile(terraformFile, []byte(meta.buildTerraformConfig("")), 0644); err != nil {
				return nil, fmt.Errorf("error creating terraform config: %w", err)
			}
			if meta.devProvider {
				meta.Logger().Debug(`Skip running "terraform init" for the import directory (dev provider)`, "dir", meta.importBaseDirs[i])
			} else {
				meta.Logger().Debug(`Run "terraform init" for the import directory`, "dir", meta.importBaseDirs[i])
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
		meta.Logger().Info("Skipping resource", "tf_id", item.TFResourceId)
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
		meta.Logger().Error("Failed to generate resource template file", "error", err, "tf_addr", item.TFAddr)
		item.ImportError = err
		return
	}
	defer os.Remove(cfgFile)

	// Import resources
	addr := item.TFAddr.String()
	if meta.moduleAddr != "" {
		addr = meta.moduleAddr + "." + addr
	}

	meta.Logger().Info("Importing a resource", "tf_id", item.TFResourceId, "tf_addr", addr)
	// The actual resource type names in telemetry is redacted
	meta.tc.Trace(telemetry.Info, fmt.Sprintf("Importing %s as %s", item.AzureResourceID.TypeString(), addr))

	err := tf.Import(ctx, addr, item.TFResourceId)
	if err != nil {
		meta.Logger().Error("Terraform import failed", "tf_addr", item.TFAddr, "error", err)
		meta.tc.Trace(telemetry.Error, fmt.Sprintf("Importing %s failed", item.AzureResourceID.TypeString()))
		meta.tc.Trace(telemetry.Error, fmt.Sprintf("Error detail: %v", err))
	} else {
		meta.tc.Trace(telemetry.Info, fmt.Sprintf("Importing %s as %s successfully", item.AzureResourceID.TypeString(), addr))
	}
	item.ImportError = err
	item.Imported = err == nil
}

func (meta *baseMeta) importItem_notf(ctx context.Context, item *ImportItem, importIdx int) {
	// Import resources
	addr := item.TFAddr.String()
	meta.Logger().Debug("Importing a resource", "tf_id", item.TFResourceId, "tf_addr", addr)
	// The actual resource type names in telemetry is redacted
	meta.tc.Trace(telemetry.Info, fmt.Sprintf("Importing %s as %s", item.AzureResourceID.TypeString(), addr))

	importResp, diags := meta.tfclient.ImportResourceState(ctx, typ.ImportResourceStateRequest{
		TypeName: item.TFAddr.Type,
		ID:       item.TFResourceId,
	})
	if diags.HasErrors() {
		meta.Logger().Error("Terraform import failed", "tf_addr", item.TFAddr, "error", diags.Err())
		meta.tc.Trace(telemetry.Error, fmt.Sprintf("Importing %s failed", item.AzureResourceID.TypeString()))
		meta.tc.Trace(telemetry.Error, fmt.Sprintf("Error detail: %v", diags.Err()))
		item.ImportError = diags.Err()
		item.Imported = false
		return
	}
	if len(importResp.ImportedResources) != 1 {
		err := fmt.Errorf("expect 1 resource being imported, got=%d", len(importResp.ImportedResources))
		meta.Logger().Error(err.Error())
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
		meta.Logger().Error("Terraform read a resource failed", "tf_addr", item.TFAddr, "error", diags.Err())
		meta.tc.Trace(telemetry.Error, fmt.Sprintf("Reading %s failed", item.AzureResourceID.TypeString()))
		meta.tc.Trace(telemetry.Error, fmt.Sprintf("Error detail: %v", diags.Err()))
		item.ImportError = diags.Err()
		item.Imported = false
		return
	}

	// Ensure the state is not null
	if readResp.NewState.IsNull() {
		meta.Logger().Error("Cannot import an non-existent resource", "tf_addr", item.TFAddr)
		meta.tc.Trace(telemetry.Error, fmt.Sprintf("Cannot import an non-existent resource: %s", item.AzureResourceID.TypeString()))
		item.ImportError = fmt.Errorf("Cannot import non-existent remote object")
		item.Imported = false
		return
	}

	meta.Logger().Debug("Finish importing a resource", "tf_id", item.TFResourceId, "tf_addr", addr)
	item.State = readResp.NewState
	item.ImportError = nil
	item.Imported = true
	return
}

func (meta baseMeta) stateToConfig(ctx context.Context, list ImportList) (ConfigInfos, error) {
	var out []ConfigInfo
	var bs [][]byte

	importedList := list.Imported()

	providerName := "registry.terraform.io/hashicorp/azurerm"
	if meta.useAzAPI() {
		providerName = "registry.terraform.io/azure/azapi"
	}

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
					ProviderName: providerName,
					Value:        item.State,
				},
				tfadd.Full(meta.fullConfig),
				tfadd.MaskSenstitive(meta.maskSensitive),
			)
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
		bs, err = tfadd.StateForTargets(ctx, meta.tf, addrs, tfadd.Full(meta.fullConfig), tfadd.MaskSenstitive(meta.maskSensitive))
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
