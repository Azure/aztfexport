package config

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
)

type OutputFileNames struct {
	// The filename for the generated "terraform.tf" (default)
	TerraformFileName string
	// The filename for the generated "provider.tf" (default)
	ProviderFileName string
	// The filename for the generated "main.tf" (default)
	MainFileName string
}

type CommonConfig struct {
	// SubscriptionId specifies the user's Azure subscription id.
	SubscriptionId string
	// AzureSDKCredential specifies the Azure SDK token credential
	AzureSDKCredential azcore.TokenCredential
	// AzureSDKClientOption specifies the Azure SDK client option
	AzureSDKClientOption arm.ClientOptions
	// OutputDir specifies the Terraform working directory for aztfy to import resources and generate TF configs.
	OutputDir string
	// OutputFileNames specifies the output terraform filenames
	OutputFileNames OutputFileNames
	// DevProvider specifies whether users have configured the `dev_overrides` for the provider, which then uses a development provider built locally rather than using a version pinned provider from official Terraform registry.
	// Meanwhile, it will also avoid running `terraform init` during `Init()` for the import directories to avoid caculating the provider hash and populating the lock file (See: https://developer.hashicorp.com/terraform/language/files/dependency-lock). Though the init for the output directory is still needed for initializing the backend.
	DevProvider bool
	// ContinueOnError specifies whether continue the progress even hit an import error.
	ContinueOnError bool
	// BackendType specifies the Terraform backend type.
	BackendType string
	// BackendConfig specifies an array of Terraform backend configs.
	BackendConfig []string
	// ProviderConfig specifies key value pairs that will be expanded to the terraform-provider-azurerm settings (i.e. `azurerm {}` block)
	// Currently, only the top level attribute whose type is string is supported.
	// This is not used directly by aztfy binary as the provider configs can be set by environment variable already.
	// While it is useful for module users that want support multi-users scenarios in one process (in which case changing env vars affect the whole process).
	ProviderConfig map[string]string
	// FullConfig specifies whether to export all (non computed-only) Terarform properties when generating TF configs.
	FullConfig bool
	// Parallelism specifies the parallelism for the process
	Parallelism int
	// ModulePath specifies the path of the module (e.g. "module1.module2") where the resources will be imported and config generated.
	// Note that only modules whose "source" is local path is supported. By default, it is the root module.
	ModulePath string
	// HCLOnly is a strange field, which is only used internally by aztfy to indicate whether to remove other files other than TF config at the end.
	// External Go modules shoudl just ignore it.
	HCLOnly bool
}

type Config struct {
	CommonConfig

	// Exactly one of below is specified

	// ResourceId specifies the Azure resource id, this indicates the resource mode.
	ResourceId string
	// ResourceGroupName specifies the name of the resource group, this indicates the resource group mode.
	ResourceGroupName string
	// ARGPredicate specifies the ARG where predicate, this indicates the query mode.
	ARGPredicate string
	// MappingFile specifies the path of mapping file, this indicates the map file mode.
	MappingFile string

	// ResourceNamePattern specifies the resource name pattern, this only applies to resource group mode and query mode.
	ResourceNamePattern string

	// RecursiveQuery specifies whether to recursively list the child/proxy resources of the ARG resulted resource list, this only applies to query mode.
	RecursiveQuery bool

	// TFResourceName specifies the TF resource name, this only applies to resource mode.
	TFResourceName string
	// TFResourceName specifies the TF resource type (if empty, aztfy will deduce the type), this only applies to resource mode.
	TFResourceType string
}
