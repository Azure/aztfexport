package meta

import (
	"testing"

	"github.com/Azure/aztfexport/internal/resourceset"
	"github.com/magodo/armid"
)

func TestSplitOriginalAndExtension(t *testing.T) {
	tests := []struct {
		name         string
		queried      []resourceset.TFResource
		requested    []resourceset.AzureResource
		expectedOrig []resourceset.TFResource
		expectedExt  []resourceset.TFResource
	}{
		{
			name:         "empty inputs",
			queried:      []resourceset.TFResource{},
			requested:    []resourceset.AzureResource{},
			expectedOrig: nil,
			expectedExt:  nil,
		},
		{
			name: "all queried resources are original",
			queried: []resourceset.TFResource{
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/storage1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/storage1",
					TFType:  "azurerm_storage_account",
				},
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1",
					TFType:  "azurerm_linux_virtual_machine",
				},
			},
			requested: []resourceset.AzureResource{
				{
					Id: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/storage1"),
				},
				{
					Id: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1"),
				},
			},
			expectedOrig: []resourceset.TFResource{
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/storage1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/storage1",
					TFType:  "azurerm_storage_account",
				},
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1",
					TFType:  "azurerm_linux_virtual_machine",
				},
			},
			expectedExt: nil,
		},
		{
			name: "mixed original and extension resources",
			queried: []resourceset.TFResource{
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/storage1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/storage1",
					TFType:  "azurerm_storage_account",
				},
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Network/networkInterfaces/nic1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Network/networkInterfaces/nic1",
					TFType:  "azurerm_network_interface",
				},
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1",
					TFType:  "azurerm_linux_virtual_machine",
				},
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Network/publicIPAddresses/pip1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Network/publicIPAddresses/pip1",
					TFType:  "azurerm_public_ip",
				},
			},
			requested: []resourceset.AzureResource{
				{
					Id: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/storage1"),
				},
				{
					Id: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1"),
				},
			},
			expectedOrig: []resourceset.TFResource{
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/storage1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/storage1",
					TFType:  "azurerm_storage_account",
				},
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1",
					TFType:  "azurerm_linux_virtual_machine",
				},
			},
			expectedExt: []resourceset.TFResource{
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Network/networkInterfaces/nic1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Network/networkInterfaces/nic1",
					TFType:  "azurerm_network_interface",
				},
				{
					AzureId: mustParseResourceId("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Network/publicIPAddresses/pip1"),
					TFId:    "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg1/providers/Microsoft.Network/publicIPAddresses/pip1",
					TFType:  "azurerm_public_ip",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actualOrig, actualExt := splitOriginalAndExtension(tc.queried, tc.requested)

			if !equalTFResources(actualOrig, tc.expectedOrig) {
				t.Errorf("splitOriginalAndExtension() original = %v, expected = %v", actualOrig, tc.expectedOrig)
			}
			if !equalTFResources(actualExt, tc.expectedExt) {
				t.Errorf("splitOriginalAndExtension() extension = %v, expected = %v", actualExt, tc.expectedExt)
			}
		})
	}
}

func mustParseResourceId(id string) armid.ResourceId {
	parsed, err := armid.ParseResourceId(id)
	if err != nil {
		panic(err)
	}
	return parsed
}

func equalTFResources(a, b []resourceset.TFResource) bool {
	if len(a) != len(b) {
		return false
	}

	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	for i := range a {
		if a[i].AzureId.String() != b[i].AzureId.String() ||
			a[i].TFId != b[i].TFId ||
			a[i].TFType != b[i].TFType {
			return false
		}
	}
	return true
}
