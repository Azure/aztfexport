package cases

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/aztfy/internal/test"

	"github.com/Azure/aztfy/internal/client"
	"github.com/Azure/aztfy/internal/resmap"
)

var _ Case = CaseKeyVaultNestedItems{}

type CaseKeyVaultNestedItems struct{}

func (CaseKeyVaultNestedItems) Tpl(d test.Data) string {
	return fmt.Sprintf(`
provider "azurerm" {
  features {
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
  }
}
resource "azurerm_resource_group" "test" {
  name     = "%[1]s"
  location = "WestEurope"
}
data "azurerm_client_config" "current" {}
resource "azurerm_key_vault" "test" {
  location                   = azurerm_resource_group.test.location
  name                       = "aztfy-test-%[2]s"
  resource_group_name        = azurerm_resource_group.test.name
  sku_name                   = "standard"
  soft_delete_retention_days = 7
  tenant_id                  = data.azurerm_client_config.current.tenant_id
  access_policy {
    tenant_id = data.azurerm_client_config.current.tenant_id
    object_id = data.azurerm_client_config.current.object_id
    certificate_permissions = [
      "Create",
      "Delete",
      "Get",
      "Import",
      "Purge",
      "Recover",
      "Update",
    ]
    key_permissions = [
      "Create",
      "Delete",
      "Get",
      "Purge",
      "Recover",
      "Update",
    ]
    secret_permissions = [
      "Set",
      "Delete",
      "Get",
      "Purge",
      "List",
      "Recover",
    ]
  }
}
resource "azurerm_key_vault_certificate" "test" {
  name         = "cert-%[2]s"
  key_vault_id = azurerm_key_vault.test.id
  certificate_policy {
    issuer_parameters {
      name = "Self"
    }
    key_properties {
      exportable = true
      key_size   = 2048
      key_type   = "RSA"
      reuse_key  = true
    }
    lifetime_action {
      action {
        action_type = "AutoRenew"
      }

      trigger {
        days_before_expiry = 30
      }
    }
    secret_properties {
      content_type = "application/x-pkcs12"
    }
    x509_certificate_properties {
      key_usage = [
        "cRLSign",
        "dataEncipherment",
        "digitalSignature",
        "keyAgreement",
        "keyEncipherment",
        "keyCertSign",
      ]
      subject            = "CN=hello-world"
      validity_in_months = 12
    }
  }
}
resource "azurerm_key_vault_secret" "test" {
  key_vault_id = azurerm_key_vault.test.id
  name         = "secret-%[2]s"
  value        = "rick-and-morty"
}
resource "azurerm_key_vault_key" "test" {
  key_vault_id = azurerm_key_vault.test.id
  name         = "key-%[2]s"
  key_opts     = ["sign", "verify"]
  key_type     = "EC"
}
`, d.RandomRgName(), d.RandomStringOfLength(8))
}

func (CaseKeyVaultNestedItems) getItems(d test.Data) (keyId, secretId, certId string, err error) {
	b, err := client.NewClientBuilder()
	if err != nil {
		return "", "", "", err
	}
	subid := os.Getenv("ARM_SUBSCRIPTION_ID")
	ctx := context.Background()
	{
		client, err := b.NewKeyvaultKeysClient(subid)
		if err != nil {
			return "", "", "", err
		}
		resp, err := client.Get(ctx, d.RandomRgName(), "aztfy-test-"+d.RandomStringOfLength(8), "key-"+d.RandomStringOfLength(8), nil)
		if err != nil {
			return "", "", "", fmt.Errorf("retrieving the key: %v", err)
		}
		if resp.Key.Properties == nil || resp.Key.Properties.KeyURIWithVersion == nil {
			return "", "", "", fmt.Errorf("failed to get data plane URI from the response for key")
		}
		keyId = *resp.Key.Properties.KeyURIWithVersion
	}
	{
		client, err := b.NewKeyvaultSecretsClient(subid)
		if err != nil {
			return "", "", "", err
		}
		resp, err := client.Get(ctx, d.RandomRgName(), "aztfy-test-"+d.RandomStringOfLength(8), "secret-"+d.RandomStringOfLength(8), nil)
		if err != nil {
			return "", "", "", fmt.Errorf("retrieving the secret: %v", err)
		}
		if resp.Secret.Properties == nil || resp.Secret.Properties.SecretURIWithVersion == nil {
			return "", "", "", fmt.Errorf("failed to get data plane URI from the response for secret")
		}
		secretId = *resp.Secret.Properties.SecretURIWithVersion
	}
	{
		client, err := b.NewKeyvaultSecretsClient(subid)
		if err != nil {
			return "", "", "", err
		}
		resp, err := client.Get(ctx, d.RandomRgName(), "aztfy-test-"+d.RandomStringOfLength(8), "cert-"+d.RandomStringOfLength(8), nil)
		if err != nil {
			return "", "", "", fmt.Errorf("retrieving the cert (secret): %v", err)
		}
		if resp.Secret.Properties == nil || resp.Secret.Properties.SecretURIWithVersion == nil {
			return "", "", "", fmt.Errorf("failed to get data plane URI from the response for cert (secret)")
		}
		id := *resp.Secret.Properties.SecretURIWithVersion
		segs := strings.Split(id, "/")
		segs[len(segs)-3] = "certificates"
		certId = strings.Join(segs, "/")
	}
	return keyId, secretId, certId, nil
}

func (c CaseKeyVaultNestedItems) ResourceMapping(d test.Data) (resmap.ResourceMapping, error) {
	keyId, secretId, certId, err := c.getItems(d)
	if err != nil {
		return nil, err
	}
	return test.ResourceMapping(fmt.Sprintf(`{
{{ "/subscriptions/%[1]s/resourcegroups/%[2]s" | ToUpper | Quote }}: {
  "resource_type": "azurerm_resource_group",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s"
},

{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.keyvault/vaults/aztfy-test-%[3]s" | ToUpper | Quote }}: {
  "resource_type": "azurerm_key_vault",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.KeyVault/vaults/aztfy-test-%[3]s"
},

{{  "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.keyvault/vaults/aztfy-test-%[3]s/keys/key-%[3]s" | ToUpper | Quote }} : {
  "resource_type": "azurerm_key_vault_key",
  "resource_name": "test",
  "resource_id": %[4]q
},

{{  "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.keyvault/vaults/aztfy-test-%[3]s/secrets/secret-%[3]s" | ToUpper | Quote }} : {
  "resource_type": "azurerm_key_vault_secret",
  "resource_name": "test",
  "resource_id": %[5]q
},

{{  "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.keyvault/vaults/aztfy-test-%[3]s/certificates/cert-%[3]s" | ToUpper | Quote }} : {
  "resource_type": "azurerm_key_vault_certificate",
  "resource_name": "test",
  "resource_id": %[6]q
}

}
`, d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8), keyId, secretId, certId))
}

func (c CaseKeyVaultNestedItems) SingleResourceContext(d test.Data) ([]SingleResourceContext, error) {
	keyId, secretId, certId, err := c.getItems(d)
	if err != nil {
		return nil, err
	}
	var (
		keyIdSuffix    string
		secretIdSuffix string
		certIdSuffix   string
	)
	{
		segs := strings.Split(keyId, "/")
		keyIdSuffix = strings.Join(segs[len(segs)-3:len(segs)-1], "/")
	}
	{
		segs := strings.Split(secretId, "/")
		secretIdSuffix = strings.Join(segs[len(segs)-3:len(segs)-1], "/")
	}
	{
		segs := strings.Split(certId, "/")
		certIdSuffix = strings.Join(segs[len(segs)-3:len(segs)-1], "/")
	}
	return []SingleResourceContext{
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s", d.SubscriptionId, d.RandomRgName()),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.KeyVault/vaults/aztfy-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.KeyVault/vaults/aztfy-test-%[3]s/%[4]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8), keyIdSuffix),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.KeyVault/vaults/aztfy-test-%[3]s/%[4]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8), secretIdSuffix),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.KeyVault/vaults/aztfy-test-%[3]s/%[4]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8), certIdSuffix),
			ExpectResourceCount: 1,
		},
	}, nil
}
