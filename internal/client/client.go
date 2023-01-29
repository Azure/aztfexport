package client

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

type ClientBuilder struct {
	Credential azcore.TokenCredential
	Opt        arm.ClientOptions
}

func (b *ClientBuilder) NewKeyvaultKeysClient(subscriptionId string) (*armkeyvault.KeysClient, error) {
	return armkeyvault.NewKeysClient(
		subscriptionId,
		b.Credential,
		&b.Opt,
	)
}

func (b *ClientBuilder) NewKeyvaultSecretsClient(subscriptionId string) (*armkeyvault.SecretsClient, error) {
	return armkeyvault.NewSecretsClient(
		subscriptionId,
		b.Credential,
		&b.Opt,
	)
}

func (b *ClientBuilder) NewResourcesClient(subscriptionId string) (*armresources.Client, error) {
	return armresources.NewClient(
		subscriptionId,
		b.Credential,
		&b.Opt,
	)
}
