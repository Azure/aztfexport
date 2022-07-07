package client

import (
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

type ClientBuilder struct {
	credential azcore.TokenCredential
	opt        *arm.ClientOptions
}

func NewClientBuilder() (*ClientBuilder, error) {
	env := "public"
	if v := os.Getenv("ARM_ENVIRONMENT"); v != "" {
		env = v
	}

	var cloudCfg cloud.Configuration
	switch strings.ToLower(env) {
	case "public":
		cloudCfg = cloud.AzurePublic
	case "usgovernment":
		cloudCfg = cloud.AzureGovernment
	case "china":
		cloudCfg = cloud.AzureChina
	default:
		return nil, fmt.Errorf("unknown environment specified: %q", env)
	}

	// Maps the auth related environment variables used in the provider to what azidentity honors.
	os.Setenv("AZURE_TENANT_ID", os.Getenv("ARM_TENANT_ID"))
	os.Setenv("AZURE_CLIENT_ID", os.Getenv("ARM_CLIENT_ID"))
	os.Setenv("AZURE_CLIENT_SECRET", os.Getenv("ARM_CLIENT_SECRET"))
	os.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", os.Getenv("ARM_CLIENT_CERTIFICATE_PATH"))

	cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloudCfg,
		},
		TenantID: os.Getenv("ARM_TENANT_ID"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a credential: %v", err)
	}

	return &ClientBuilder{
		credential: cred,
		opt: &arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloudCfg,
				Telemetry: policy.TelemetryOptions{
					ApplicationID: "aztfy",
					Disabled:      false,
				},
				Logging: policy.LogOptions{
					IncludeBody: true,
				},
			},
		},
	}, nil
}

func (b *ClientBuilder) NewResourceGroupClient(subscriptionId string) (*armresources.ResourceGroupsClient, error) {
	return armresources.NewResourceGroupsClient(
		subscriptionId,
		b.credential,
		b.opt,
	)
}

func (b *ClientBuilder) NewKeyvaultKeysClient(subscriptionId string) (*armkeyvault.KeysClient, error) {
	return armkeyvault.NewKeysClient(
		subscriptionId,
		b.credential,
		b.opt,
	)
}

func (b *ClientBuilder) NewKeyvaultSecretsClient(subscriptionId string) (*armkeyvault.SecretsClient, error) {
	return armkeyvault.NewSecretsClient(
		subscriptionId,
		b.credential,
		b.opt,
	)
}
