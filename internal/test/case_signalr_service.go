package test

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/aztfy/internal/resmap"
)

type CaseSignalRService struct{}

func (CaseSignalRService) Tpl(d Data) string {
	return fmt.Sprintf(`
provider "azurerm" {
  features {}
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

func (CaseSignalRService) ResourceMapping(d Data) (resmap.ResourceMapping, error) {
	rm := fmt.Sprintf(`{
"/subscriptions/%[1]s/resourceGroups/%[2]s": "azurerm_resource_group.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.SignalRService/signalR/test-%[3]s": "azurerm_signalr_service.test"
}
`, d.subscriptionId, d.RandomRgName(), d.RandomStringOfLength(8))
	m := resmap.ResourceMapping{}
	if err := json.Unmarshal([]byte(rm), &m); err != nil {
		return nil, err
	}
	return m, nil
}
