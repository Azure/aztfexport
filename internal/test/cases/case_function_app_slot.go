package cases

import (
	"fmt"

	"github.com/Azure/aztfexport/internal/test"

	"github.com/Azure/aztfexport/internal/resmap"
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
  name                     = "aztfexporttest%[2]s"
  resource_group_name      = azurerm_resource_group.test.name
  location                 = azurerm_resource_group.test.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}
resource "azurerm_service_plan" "test" {
  name                = "aztfexport-test-%[2]s"
  location            = azurerm_resource_group.test.location
  resource_group_name = azurerm_resource_group.test.name
  os_type             = "Windows"
  sku_name            = "Y1"

}
resource "azurerm_windows_function_app" "test" {
  name                       = "aztfexport-test-%[2]s"
  location                   = azurerm_resource_group.test.location
  resource_group_name        = azurerm_resource_group.test.name
  service_plan_id            = azurerm_service_plan.test.id
  storage_account_name       = azurerm_storage_account.test.name
  storage_account_access_key = azurerm_storage_account.test.primary_access_key
  site_config {}
}
resource "azurerm_windows_function_app_slot" "test" {
  name                       = "aztfexport-test-%[2]s"
  function_app_id            = azurerm_windows_function_app.test.id
  storage_account_name       = azurerm_storage_account.test.name
  storage_account_access_key = azurerm_storage_account.test.primary_access_key
  site_config {}
}
`, d.RandomRgName(), d.RandomStringOfLength(8))
}

func (CaseFunctionAppSlot) Total() int {
	return 5
}

func (CaseFunctionAppSlot) ResourceMapping(d test.Data) (resmap.ResourceMapping, error) {
	return test.ResourceMapping(fmt.Sprintf(`{
{{ "/subscriptions/%[1]s/resourcegroups/%[2]s" | Quote }}: {
  "resource_type": "azurerm_resource_group",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s"
},

{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.storage/storageaccounts/aztfexporttest%[3]s" | Quote }}: {
  "resource_type": "azurerm_storage_account",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Storage/storageAccounts/aztfexporttest%[3]s"
},

{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.web/serverfarms/aztfexport-test-%[3]s" | Quote }}: {
  "resource_type": "azurerm_service_plan",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/serverfarms/aztfexport-test-%[3]s"
},

{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.web/sites/aztfexport-test-%[3]s" | Quote }}: {
  "resource_type": "azurerm_windows_function_app",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/sites/aztfexport-test-%[3]s"
},

{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.web/sites/aztfexport-test-%[3]s/slots/aztfexport-test-%[3]s" | Quote }}: {
  "resource_type": "azurerm_windows_function_app_slot",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/sites/aztfexport-test-%[3]s/slots/aztfexport-test-%[3]s"
}

}
`, d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)))
}

func (CaseFunctionAppSlot) SingleResourceContext(d test.Data) ([]SingleResourceContext, error) {
	return []SingleResourceContext{
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s", d.SubscriptionId, d.RandomRgName()),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Storage/storageAccounts/aztfexporttest%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/serverfarms/aztfexport-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/sites/aztfexport-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Web/sites/aztfexport-test-%[3]s/slots/aztfexport-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
	}, nil
}
