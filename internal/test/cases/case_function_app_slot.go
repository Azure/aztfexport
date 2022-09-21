package cases

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/aztfy/internal/test"

	"github.com/Azure/aztfy/internal/resmap"
)

var _ Case = CaseFunctionAppSlot{}

type CaseFunctionAppSlot struct{}

func (CaseFunctionAppSlot) Tpl(d test.Data) string {
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
  location = "EastUS2"
}
resource "azurerm_storage_account" "test" {
  name                     = "aztfytest%[2]s"
  resource_group_name      = azurerm_resource_group.test.name
  location                 = azurerm_resource_group.test.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}
resource "azurerm_service_plan" "test" {
  name                = "aztfy-test-%[2]s"
  location            = azurerm_resource_group.test.location
  resource_group_name = azurerm_resource_group.test.name
  os_type             = "Windows"
  sku_name            = "Y1"

}
resource "azurerm_windows_function_app" "test" {
  name                       = "aztfy-test-%[2]s"
  location                   = azurerm_resource_group.test.location
  resource_group_name        = azurerm_resource_group.test.name
  service_plan_id            = azurerm_service_plan.test.id
  storage_account_name       = azurerm_storage_account.test.name
  storage_account_access_key = azurerm_storage_account.test.primary_access_key
  site_config {}
}
resource "azurerm_windows_function_app_slot" "test" {
  name                       = "aztfy-test-%[2]s"
  function_app_id            = azurerm_windows_function_app.test.id
  storage_account_name       = azurerm_storage_account.test.name
  storage_account_access_key = azurerm_storage_account.test.primary_access_key
  site_config {}
}
`, d.RandomRgName(), d.RandomStringOfLength(8))
}

func (CaseFunctionAppSlot) ResourceMapping(d test.Data) (resmap.ResourceMapping, error) {
	rm := fmt.Sprintf(`{
"/subscriptions/%[1]s/resourceGroups/%[2]s": "azurerm_resource_group.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Storage/storageAccounts/aztfytest%[3]s": "azurerm_storage_account.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/serverfarms/aztfy-test-%[3]s": "azurerm_service_plan.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/sites/aztfy-test-%[3]s": "azurerm_windows_function_app.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/sites/aztfy-test-%[3]s/slots/aztfy-test-%[3]s": "azurerm_windows_function_app_slot.test"
}
`, d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8))
	m := resmap.ResourceMapping{}
	if err := json.Unmarshal([]byte(rm), &m); err != nil {
		return nil, err
	}
	return m, nil
}
func (CaseFunctionAppSlot) AzureResourceIds(d test.Data) ([]string, error) {
	return []string{
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s", d.SubscriptionId, d.RandomRgName()),
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Storage/storageAccounts/aztfytest%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/serverfarms/aztfy-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/sites/aztfy-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/sites/aztfy-test-%[3]s/slots/aztfy-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
	}, nil
}
