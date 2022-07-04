package resourcegroup

import (
	"encoding/json"
	"fmt"
	"github.com/Azure/aztfy/internal/test"

	"github.com/Azure/aztfy/internal/resmap"
)

type CaseStorageFileShare struct{}

func (CaseStorageFileShare) Tpl(d test.Data) string {
	return fmt.Sprintf(`
provider "azurerm" {
  features {}
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
	rm := fmt.Sprintf(`{
"/subscriptions/%[1]s/resourceGroups/%[2]s": "azurerm_resource_group.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Storage/storageAccounts/aztfy%[3]s": "azurerm_storage_account.test",
"https://aztfy%[3]s.file.core.windows.net/aztfy%[3]s": "azurerm_storage_share.test"
}
`, d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8))
	m := resmap.ResourceMapping{}
	if err := json.Unmarshal([]byte(rm), &m); err != nil {
		return nil, err
	}
	return m, nil
}
