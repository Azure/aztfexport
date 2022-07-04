package cases

import (
	"encoding/json"
	"fmt"
	"github.com/Azure/aztfy/internal/test"

	"github.com/Azure/aztfy/internal/resmap"
)

var _ Case = CaseComputeVMDisk{}

type CaseComputeVMDisk struct{}

func (CaseComputeVMDisk) Tpl(d test.Data) string {
	return fmt.Sprintf(`
provider "azurerm" {
  features {}
}
resource "azurerm_resource_group" "test" {
  name     = "%[1]s"
  location = "WestEurope"
}
resource "azurerm_virtual_network" "test" {
  name                = "aztfy-test-%[2]s"
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
resource "azurerm_network_interface" "test" {
  name                = "aztfy-test-%[2]s"
  location            = azurerm_resource_group.test.location
  resource_group_name = azurerm_resource_group.test.name
  ip_configuration {
    name                          = "testconfiguration1"
    subnet_id                     = azurerm_subnet.test.id
    private_ip_address_allocation = "Dynamic"
  }
}
resource "azurerm_linux_virtual_machine" "test" {
  name                            = "aztfy-test-%[2]s"
  resource_group_name = azurerm_resource_group.test.name
  location            = azurerm_resource_group.test.location
  size                = "Standard_F2"
  admin_username      = "adminuser"
  network_interface_ids = [
    azurerm_network_interface.test.id,
  ]
  admin_ssh_key {
    username   = "adminuser"
    public_key = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC+wWK73dCr+jgQOAxNsHAnNNNMEMWOHYEccp6wJm2gotpr9katuF/ZAdou5AaW1C61slRkHRkpRRX9FA9CYBiitZgvCCz+3nWNN7l/Up54Zps/pHWGZLHNJZRYyAB6j5yVLMVHIHriY49d/GZTZVNB8GoJv9Gakwc/fuEZYYl4YDFiGMBP///TzlI4jhiJzjKnEvqPFki5p2ZRJqcbCiF4pJrxUQR/RXqVFQdbRLZgYfJ8xGB878RENq3yQ39d8dVOkq4edbkzwcUmwwwkYVPIoDGsYLaRHnG+To7FvMeyO7xDVQkMKzopTQV8AuKpyvpqu0a9pWOMaiCyDytO7GGN you@me.com"
  }
  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }
  source_image_reference {
    publisher = "Canonical"
    offer     = "UbuntuServer"
    sku       = "16.04-LTS"
    version   = "latest"
  }
}
resource "azurerm_managed_disk" "test" {
  name                 = "aztfy-test-%[2]s"
  location             = azurerm_resource_group.test.location
  resource_group_name  = azurerm_resource_group.test.name
  storage_account_type = "Standard_LRS"
  create_option        = "Empty"
  disk_size_gb         = 10
}
resource "azurerm_virtual_machine_data_disk_attachment" "test" {
  managed_disk_id    = azurerm_managed_disk.test.id
  virtual_machine_id = azurerm_linux_virtual_machine.test.id
  lun                = "0"
  caching            = "None"
}
`, d.RandomRgName(), d.RandomStringOfLength(8))
}

func (CaseComputeVMDisk) ResourceMapping(d test.Data) (resmap.ResourceMapping, error) {
	rm := fmt.Sprintf(`{
"/subscriptions/%[1]s/resourceGroups/%[2]s": "azurerm_resource_group.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Compute/disks/aztfy-test-%[3]s": "azurerm_managed_disk.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Compute/virtualMachines/aztfy-test-%[3]s": "azurerm_linux_virtual_machine.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Network/networkInterfaces/aztfy-test-%[3]s": "azurerm_network_interface.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Network/virtualNetworks/aztfy-test-%[3]s": "azurerm_virtual_network.test",
"/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Network/virtualNetworks/aztfy-test-%[3]s/subnets/internal": "azurerm_subnet.test"
}
`, d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8))
	m := resmap.ResourceMapping{}
	if err := json.Unmarshal([]byte(rm), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (CaseComputeVMDisk) AzureResourceIds(d test.Data) []string {
	return []string{
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s", d.SubscriptionId, d.RandomRgName()),
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Compute/disks/aztfy-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Compute/virtualMachines/aztfy-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Network/networkInterfaces/aztfy-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Network/virtualNetworks/aztfy-test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
		fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Network/virtualNetworks/aztfy-test-%[3]s/subnets/internal", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
	}
}
