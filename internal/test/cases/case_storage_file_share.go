package cases

import (
	"fmt"

	"github.com/Azure/aztfy/internal/test"

	"github.com/Azure/aztfy/internal/resmap"
)

var _ Case = CaseStorageFileShare{}

type CaseStorageFileShare struct{}

func (CaseStorageFileShare) Tpl(d test.Data) string {
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

resource "azurerm_storage_account" "test" {
  name                     = "aztfy%[2]s"
  resource_group_name      = azurerm_resource_group.test.name
  location                 = azurerm_resource_group.test.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}
resource "azurerm_storage_share" "test" {
  name                 = "aztfy%[2]s"
  storage_account_name = azurerm_storage_account.test.name
  quota                = 5
}
`, d.RandomRgName(), d.RandomStringOfLength(8))
}

func (CaseStorageFileShare) ResourceMapping(d test.Data) (resmap.ResourceMapping, error) {
	return test.ResourceMapping(fmt.Sprintf(`{
{{ "/subscriptions/%[1]s/resourcegroups/%[2]s" | Quote }}: {
  "resource_type": "azurerm_resource_group",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s"
},

{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.storage/storageaccounts/aztfy%[3]s" | Quote }}: {
  "resource_type": "azurerm_storage_account",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Storage/storageAccounts/aztfy%[3]s"
},

{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.storage/storageaccounts/aztfy%[3]s/fileservices/default/shares/aztfy%[3]s" | Quote }}: {
  "resource_type": "azurerm_storage_share",
  "resource_name": "test",
  "resource_id": "https://aztfy%[3]s.file.core.windows.net/aztfy%[3]s"
}

}
`, d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)))
}

func (CaseStorageFileShare) SingleResourceContext(d test.Data) ([]SingleResourceContext, error) {
	return []SingleResourceContext{
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s", d.SubscriptionId, d.RandomRgName()),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Storage/storageAccounts/aztfy%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Storage/storageAccounts/aztfy%[3]s/fileServices/default/shares/aztfy%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
	}, nil
}
