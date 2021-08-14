package internal

import (
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-06-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/hashicorp/go-azure-helpers/authentication"
	"github.com/hashicorp/go-azure-helpers/sender"
)

type Authorizer struct {
	authorizer autorest.Authorizer
	Config     *authentication.Config
}

func NewAuthorizer() (*Authorizer, error) {
	builder := &authentication.Builder{
		SubscriptionID:     os.Getenv("ARM_SUBSCRIPTION_ID"),
		ClientID:           os.Getenv("ARM_CLIENT_ID"),
		ClientSecret:       os.Getenv("ARM_CLIENT_SECRET"),
		TenantID:           os.Getenv("ARM_TENANT_ID"),
		Environment:        "public",
		ClientCertPassword: os.Getenv("ARM_CLIENT_CERTIFICATE_PASSWORD"),
		ClientCertPath:     os.Getenv("ARM_CLIENT_CERTIFICATE_PATH"),

		// Feature Toggles
		SupportsClientCertAuth:   true,
		SupportsClientSecretAuth: true,
		SupportsAzureCliToken:    true,
	}

	config, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("Error building AzureRM Client: %v", err)
	}

	env, err := authentication.DetermineEnvironment(config.Environment)
	if err != nil {
		return nil, fmt.Errorf("determining environment: %v", err)
	}

	oauthConfig, err := config.BuildOAuthConfig(env.ActiveDirectoryEndpoint)
	if err != nil {
		return nil, fmt.Errorf("building OAuth Config: %v", err)
	}

	// OAuthConfigForTenant returns a pointer, which can be nil.
	if oauthConfig == nil {
		return nil, fmt.Errorf("unable to configure OAuthConfig for tenant %s", config.TenantID)
	}

	sender := sender.BuildSender("AzureRM")

	auth, err := config.GetAuthorizationToken(sender, oauthConfig, env.TokenAudience)
	if err != nil {
		return nil, fmt.Errorf("unable to get authorization token for resource manager: %v", err)
	}

	return &Authorizer{
		authorizer: auth,
		Config:     config,
	}, nil
}

func (a *Authorizer) NewResourceClient() resources.Client {
	client := resources.NewClient(a.Config.SubscriptionID)
	client.Authorizer = a.authorizer
	return client
}

func (a *Authorizer) NewResourceGroupClient() resources.GroupsClient {
	client := resources.NewGroupsClient(a.Config.SubscriptionID)
	client.Authorizer = a.authorizer
	return client
}
