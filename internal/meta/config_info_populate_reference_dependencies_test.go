package meta

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPopulateReferenceDependencies(t *testing.T) {
	testCases := []struct {
		name                  string
		inputConfigs          ConfigInfos
		expectedReferenceDeps map[string]map[string]Dependency   // key: AzureResourceId, inner key: TFResourceId
		expectedAmbiguousDeps map[string]map[string][]Dependency // key: AzureResourceId, inner key: TFResourceId
	}{
		{
			name: "no dependencies between resources",
			inputConfigs: []ConfigInfo{
				configInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					tfAddr("azurerm_foo_resource.res-0"),
					`
resource "azurerm_foo_resource" "res-0" {
  name              = "foo1"
}
`,
				),
				configInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					tfAddr("azurerm_bar_resource.res-1"),
					`
resource "azurerm_bar_resource" "res-1" {
  name              = "bar1"
}
`,
				),
			},
			expectedReferenceDeps: map[string]map[string]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1": {},
			},
			expectedAmbiguousDeps: map[string]map[string][]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1": {},
			},
		},
		{
			name: "res-0 is a resource group, res-1 refer to it by id: expect reference dep from res-1 to res-0",
			inputConfigs: []ConfigInfo{
				configInfo(
					"/subscriptions/123/resourceGroups/rg1",
					"/subscriptions/123/resourceGroups/rg1",
					tfAddr("azurerm_resource_group.res-0"),
					`
resource "azurerm_resource_group" "res-0" {
  name     = "rg1"
  location = "West Europe"
}
`,
				),
				configInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					tfAddr("azurerm_foo_resource.res-1"),
					`
resource "azurerm_foo_resource" "res-1" {
  name              = "foo1"
  resource_group_id = "/subscriptions/123/resourceGroups/rg1"
}
`,
				),
			},
			expectedReferenceDeps: map[string]map[string]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {
					"/subscriptions/123/resourceGroups/rg1": {
						TFAddr:          tfAddr("azurerm_resource_group.res-0"),
						AzureResourceId: "/subscriptions/123/resourceGroups/rg1",
						TFResourceId:    "/subscriptions/123/resourceGroups/rg1",
					},
				},
				"/subscriptions/123/resourceGroups/rg1": make(map[string]Dependency),
			},
			expectedAmbiguousDeps: map[string]map[string][]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1":                                  {},
			},
		},
		{
			name: "res-0 and res-1 have different azure resource id, but same TF resource id, res-2 refer to this TF resource id: expect res-2 to have ambiguous dep to the TF resource id",
			inputConfigs: []ConfigInfo{
				configInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					tfAddr("azurerm_foo_resource.res-0"),
					`
resource "azurerm_foo_sub1_resource" "res-0" {
  name = "foo1_sub1"
}
`,
				),
				configInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					tfAddr("azurerm_foo_resource.res-1"),
					`
resource "azurerm_foo_sub2_resource" "res-1" {
  name = "foo1_sub2"
}
`,
				),
				configInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					tfAddr("azurerm_bar_resource.res-2"),
					`
resource "azurerm_bar_resource" "res-2" {
  name              = "bar1"
	foo_resource_id   = "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"
}
`,
				),
			},
			expectedReferenceDeps: map[string]map[string]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1":           {},
			},
			expectedAmbiguousDeps: map[string]map[string][]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1": {
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {
						{
							TFAddr:          tfAddr("azurerm_foo_resource.res-0"),
							AzureResourceId: "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1",
							TFResourceId:    "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
						},
						{
							TFAddr:          tfAddr("azurerm_foo_resource.res-1"),
							AzureResourceId: "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2",
							TFResourceId:    "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
						},
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
				expectedReferenceDeps := testCase.expectedReferenceDeps[cfg.AzureResourceID.String()]
				assert.Equal(t, cfg.dependencies.referenceDeps, expectedReferenceDeps, "referenceDeps matches expectation, azureResourceId: %s", cfg.AzureResourceID.String())
				expectedAmbiguousDeps := testCase.expectedAmbiguousDeps[cfg.AzureResourceID.String()]
				assert.Equal(t, cfg.dependencies.ambiguousDeps, expectedAmbiguousDeps, "ambiguousDeps matches expectation, azureResourceId: %s", cfg.AzureResourceID.String())
			}
		})
	}
}
