package cases

import (
	"fmt"

	"github.com/Azure/aztfexport/internal/resmap"
	"github.com/Azure/aztfexport/internal/test"
)

var _ Case = CaseVnet{}

type CaseVnet struct{}

func (CaseVnet) Tpl(d test.Data) string {
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
resource "azurerm_virtual_network" "test" {
  name                = "aztfexport-test-%[2]s"
  address_space       = ["10.0.0.0/16"]
  location            = azurerm_resource_group.test.location
  resource_group_name = azurerm_resource_group.test.name
}
resource "azurerm_subnet" "test" {
  name                 = "internal"
  resource_group_name  = azurerm_resource_group.test.name
  virtual_network_name = azurerm_virtual_network.test.name
  address_prefixes     = ["10.0.2.0/24"]
}
`, d.RandomRgName(), d.RandomStringOfLength(8))
}

func (CaseVnet) Total() int {
	return 3
}

func (CaseVnet) ResourceMapping(d test.Data) (resmap.ResourceMapping, error) {
	return test.ResourceMapping(fmt.Sprintf(`{
{{ "/subscriptions/%[1]s/resourcegroups/%[2]s" | Quote }}: {
  "resource_type": "azurerm_resource_group",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s"
},

{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.network/virtualnetworks/aztfexport-test-%[3]s" | Quote }}: {
  "resource_type": "azurerm_virtual_network",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Network/virtualNetworks/aztfexport-test-%[3]s"
},

{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.network/virtualnetworks/aztfexport-test-%[3]s/subnets/internal" | Quote }}: {
  "resource_type": "azurerm_subnet",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Network/virtualNetworks/aztfexport-test-%[3]s/subnets/internal"
}

}
`, d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)))
}

func (CaseVnet) SingleResourceContext(d test.Data) ([]SingleResourceContext, error) {
	return []SingleResourceContext{
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s", d.SubscriptionId, d.RandomRgName()),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Network/virtualNetworks/aztfexport-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Network/virtualNetworks/aztfexport-test-%[3]s/subnets/internal", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
	}, nil
}
