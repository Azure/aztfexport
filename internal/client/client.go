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
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
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
	if v, ok := os.LookupEnv("ARM_TENANT_ID"); ok {
		os.Setenv("AZURE_TENANT_ID", v)
	}
	if v, ok := os.LookupEnv("ARM_CLIENT_ID"); ok {
		os.Setenv("AZURE_CLIENT_ID", v)
	}
	if v, ok := os.LookupEnv("ARM_CLIENT_SECRET"); ok {
		os.Setenv("AZURE_CLIENT_SECRET", v)
	}
	if v, ok := os.LookupEnv("ARM_CLIENT_CERTIFICATE_PATH"); ok {
		os.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", v)
	}

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

func (b *ClientBuilder) NewResourceGraphClient() (*armresourcegraph.Client, error) {
	return armresourcegraph.NewClient(
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
