provider "azurerm" {
  features {}
}
resource "azurerm_resource_group" "rg" {
  name     = "aztfy"
  location = "WestEurope"
}
resource "azurerm_container_registry" "acr" {
  name                = "aztfy"
  resource_group_name = azurerm_resource_group.rg.name
  location            = azurerm_resource_group.rg.location
  sku                 = "Standard"
  admin_enabled       = true
}
