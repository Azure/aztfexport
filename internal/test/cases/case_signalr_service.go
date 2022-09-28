package cases

import (
	"fmt"

	"github.com/Azure/aztfy/internal/test"

	"github.com/Azure/aztfy/internal/resmap"
)

var _ Case = CaseSignalRService{}

type CaseSignalRService struct{}

func (CaseSignalRService) Tpl(d test.Data) string {
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
resource "azurerm_signalr_service" "test" {
  name                = "test-%[2]s"
  location            = azurerm_resource_group.test.location
  resource_group_name = azurerm_resource_group.test.name
  sku {
    name     = "Free_F1"
    capacity = 1
  }
}
`, d.RandomRgName(), d.RandomStringOfLength(8))
}

func (CaseSignalRService) ResourceMapping(d test.Data) (resmap.ResourceMapping, error) {
	return test.ResourceMapping(fmt.Sprintf(`{
{{ "/subscriptions/%[1]s/resourcegroups/%[2]s" | Quote }}: {
  "resource_type": "azurerm_resource_group",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s"
},

{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.signalrservice/signalr/test-%[3]s" | Quote }}: {
  "resource_type": "azurerm_signalr_service",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.SignalRService/signalR/test-%[3]s"
}

}
`, d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)))
}

func (CaseSignalRService) SingleResourceContext(d test.Data) ([]SingleResourceContext, error) {
	return []SingleResourceContext{
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s", d.SubscriptionId, d.RandomRgName()),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.SignalRService/signalR/test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
	}, nil
}
