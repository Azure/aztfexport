package meta

import (
	"testing"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/internal/tfresourceid"
	"github.com/stretchr/testify/assert"
)

func TestPopulateReferenceDependencies(t *testing.T) {
	testCases := []struct {
		name                  string
		inputConfigs          ConfigInfos
		expectedReferenceDeps map[AzureResourceId]ReferenceDependencies
		expectedAmbiguousDeps map[AzureResourceId]AmbiguousDependencies
	}{
		{
			name: "no dependencies between resources",
			inputConfigs: []ConfigInfo{
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"),
					tfAddr("azurerm_foo_resource.res-0"),
					`
resource "azurerm_foo_resource" "res-0" {
  name              = "foo1"
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
  name              = "bar1"
}
`,
					ReferenceDependencies{},
					AmbiguousDependencies{},
				),
			},
			expectedReferenceDeps: nil,
			expectedAmbiguousDeps: nil,
		},
		{
			name: "res-0 is a resource group, res-1 refer to it by id: expect reference dep from res-1 to res-0",
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
					ReferenceDependencies{},
					AmbiguousDependencies{},
				),
			},
			expectedReferenceDeps: map[AzureResourceId]ReferenceDependencies{
				AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"): {
					internalMap: map[tfresourceid.TFResourceId]tfaddr.TFAddr{
						tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1"): tfAddr("azurerm_resource_group.res-0"),
					},
				},
			},
			expectedAmbiguousDeps: nil,
		},
		{
			name: "res-0 and res-1 have different azure resource id, but same TF resource id, res-2 refer to this TF resource id: expect res-2 to have ambiguous dep to the TF resource id",
			inputConfigs: []ConfigInfo{
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"),
					tfAddr("azurerm_foo_resource.res-0"),
					`
resource "azurerm_foo_sub1_resource" "res-0" {
  name              = "foo1_sub1"
}
`,
					ReferenceDependencies{},
					AmbiguousDependencies{},
				),
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"),
					tfAddr("azurerm_foo_resource.res-1"),
					`
resource "azurerm_foo_sub2_resource" "res-1" {
  name              = "foo1_sub2"
}
`,
					ReferenceDependencies{},
					AmbiguousDependencies{},
				),
				configInfo(
					AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1"),
					tfAddr("azurerm_bar_resource.res-2"),
					`
resource "azurerm_bar_resource" "res-2" {
  name              = "bar1"
	foo_resource_id   = "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"
}
`,
					ReferenceDependencies{},
					AmbiguousDependencies{},
				),
			},
			expectedReferenceDeps: nil,
			expectedAmbiguousDeps: map[AzureResourceId]AmbiguousDependencies{
				AzureResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1"): {
					internalMap: map[tfresourceid.TFResourceId]*TFAddrSet{
						tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1"): tfAddrSet(
							"azurerm_foo_resource.res-0",
							"azurerm_foo_resource.res-1",
						),
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.inputConfigs.PopulateReferenceDependencies()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for _, cfg := range testCase.inputConfigs {
				azureResourceId := AzureResourceId(cfg.AzureResourceID.String())
				expectedReferenceDeps := testCase.expectedReferenceDeps[azureResourceId]
				assert.Equal(t, cfg.referenceDependencies, expectedReferenceDeps)
				expectedAmbiguousDeps := testCase.expectedAmbiguousDeps[azureResourceId]
				assert.Equal(t, cfg.ambiguousDependencies.List(), expectedAmbiguousDeps.List())
			}
		})
	}
}
