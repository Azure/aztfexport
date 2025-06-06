package meta

import (
	"testing"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/internal/tfresourceid"
	"github.com/stretchr/testify/assert"
)

func TestPopulateParentChildDependencies(t *testing.T) {
	testCases := []struct {
		name                 string
		inputConfigs         ConfigInfos
		expectedExplicitDeps map[AzureResourceId]TFAddrSet
	}{
		{
			name: "no parent-child relationships",
			inputConfigs: []ConfigInfo{
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"),
					tfAddr("azurerm_foo_resource.res-0"),
					`
resource "azurerm_foo_resource" "res-0" {
  name = "foo1"
}
`,
					ReferenceDependencies{},
					AmbiguousDependencies{},
				),
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1"),
					tfAddr("azurerm_bar_resource.res-1"),
					`
resource "azurerm_bar_resource" "res-1" {
  name = "bar1"
}
`,
					ReferenceDependencies{},
					AmbiguousDependencies{},
				),
			},
			expectedExplicitDeps: nil,
		},
		{
			name: "res-0 is a parent of res-1: expect explicit dep from res-1 to res-0",
			inputConfigs: []ConfigInfo{
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1"),
					tfAddr("azurerm_resource_group.res-0"),
					`
resource "azurerm_resource_group" "res-0" {
  name     = "rg1"
  location = "West Europe"
}
`,
					ReferenceDependencies{},
					AmbiguousDependencies{},
				),
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"),
					tfAddr("azurerm_foo_resource.res-1"),
					`
resource "azurerm_foo_resource" "res-1" {
  name                = "foo1"
	resource_group_name = "rg1"
}
`,
					ReferenceDependencies{},
					AmbiguousDependencies{},
				),
			},
			expectedExplicitDeps: map[AzureResourceId]TFAddrSet{
				AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"): *tfAddrSet(
					"azurerm_resource_group.res-0",
				),
			},
		},
		{
			name: "res-2 -> res-1 -> res-0 connected by reference dependency, res-2 is child of res-0: expect no explicit dep because it has been satisfied transitively by reference dep",
			inputConfigs: []ConfigInfo{
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1"),
					tfAddr("azurerm_resource_group.res-0"),
					`
resource "azurerm_resource_group" "res-0" {
  name     = "rg1"
  location = "West Europe"
}
`,
					ReferenceDependencies{},
					AmbiguousDependencies{},
				),
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"),
					tfAddr("azurerm_foo_resource.res-1"),
					`
resource "azurerm_foo_resource" "res-1" {
  name              = "foo1"
	resource_group_id = "/subscriptions/123/resourceGroups/rg1"
}
`,
					ReferenceDependencies{
						internalMap: map[tfresourceid.TFResourceId]tfaddr.TFAddr{
							tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1"): tfAddr("azurerm_resource_group.res-0"),
						},
					},
					AmbiguousDependencies{},
				),
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1"),
					tfAddr("azurerm_bar_resource.res-2"),
					`
resource "azurerm_bar_resource" "res-2" {
  name   = "bar1"
	foo_id = "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"
}
`,
					ReferenceDependencies{
						internalMap: map[tfresourceid.TFResourceId]tfaddr.TFAddr{
							tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1"): tfAddr("azurerm_resource_group.res-1"),
						},
					},
					AmbiguousDependencies{},
				),
			},
			expectedExplicitDeps: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.inputConfigs.populateParentChildDependency()
			for _, cfg := range testCase.inputConfigs {
				azureResourceId := AzureResourceId(cfg.AzureResourceID.String())
				expectedExplicitDeps := testCase.expectedExplicitDeps[azureResourceId]
				assert.Equal(t, cfg.explicitDependencies.List(), expectedExplicitDeps.List())
			}
		})
	}
}
