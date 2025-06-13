package meta

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPopulateParentChildDependencies(t *testing.T) {
	testCases := []struct {
		name                    string
		inputConfigs            ConfigInfos
		expectedParentChildDeps map[string]map[Dependency]bool // key: AzureResourceId
	}{
		{
			name: "no parent-child relationships",
			inputConfigs: []ConfigInfo{
				configInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					mustParseTFAddr("azurerm_foo_resource.res-0"),
					`
resource "azurerm_foo_resource" "res-0" {
  name = "foo1"
}
`,
				),
				configInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					mustParseTFAddr("azurerm_bar_resource.res-1"),
					`
resource "azurerm_bar_resource" "res-1" {
  name = "bar1"
}
`,
				),
			},
			expectedParentChildDeps: map[string]map[Dependency]bool{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1": {},
			},
		},
		{
			name: "res-0 is a parent of res-1: expect explicit dep from res-1 to res-0",
			inputConfigs: []ConfigInfo{
				configInfo(
					"/subscriptions/123/resourceGroups/rg1",
					"/subscriptions/123/resourceGroups/rg1",
					mustParseTFAddr("azurerm_resource_group.res-0"),
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
					mustParseTFAddr("azurerm_foo_resource.res-1"),
					`
resource "azurerm_foo_resource" "res-1" {
  name                = "foo1"
	resource_group_name = "rg1"
}
`,
				),
			},
			expectedParentChildDeps: map[string]map[Dependency]bool{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {
					{
						TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
						AzureResourceId: "/subscriptions/123/resourceGroups/rg1",
						TFResourceId:    "/subscriptions/123/resourceGroups/rg1",
					}: true,
				},
				"/subscriptions/123/resourceGroups/rg1": {},
			},
		},
		{
			name: "res-2 -> res-1 -> res-0 connected by reference dependency, res-2 is child of res-0: expect no explicit dep because it has been satisfied transitively by reference dep",
			inputConfigs: []ConfigInfo{
				configInfo(
					"/subscriptions/123/resourceGroups/rg1",
					"/subscriptions/123/resourceGroups/rg1",
					mustParseTFAddr("azurerm_resource_group.res-0"),
					`
resource "azurerm_resource_group" "res-0" {
  name     = "rg1"
  location = "West Europe"
}
`,
				),
				configInfoWithDeps(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					mustParseTFAddr("azurerm_foo_resource.res-1"),
					`
resource "azurerm_foo_resource" "res-1" {
  name              = "foo1"
	resource_group_id = "/subscriptions/123/resourceGroups/rg1"
}
`,
					map[string]Dependency{
						"/subscriptions/123/resourceGroups/rg1": {
							TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
							AzureResourceId: "/subscriptions/123/resourceGroups/rg1",
							TFResourceId:    "/subscriptions/123/resourceGroups/rg1",
						},
					},
					make(map[string][]Dependency),
				),
				configInfoWithDeps(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					mustParseTFAddr("azurerm_bar_resource.res-2"),
					`
resource "azurerm_bar_resource" "res-2" {
  name   = "bar1"
	foo_id = "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"
}
`,
					map[string]Dependency{
						"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1": {
							TFAddr:          mustParseTFAddr("azurerm_resource_group.res-1"),
							AzureResourceId: "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
							TFResourceId:    "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
						},
					},
					make(map[string][]Dependency),
				),
			},
			expectedParentChildDeps: map[string]map[Dependency]bool{
				"/subscriptions/123/resourceGroups/rg1":                                  {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1": {},
			},
		},
		{
			name: "res-0 and res-1 are ambiguous (different azureResourceId, same tfResourceId), res-2 is child of res-0, res-2 has ambiguous refDep to res-0 and res-1: expect parentChildDep to be added to res-2",
			inputConfigs: []ConfigInfo{
				configInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					mustParseTFAddr("azurerm_foo_sub1_resource.res-0"),
					`
resource "azurerm_foo_sub1_resource" "res-0" {
  name     = "res0"
  location = "West Europe"
}
`,
				),
				configInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					mustParseTFAddr("azurerm_foo_sub2_resource.res-1"),
					`
resource "azurerm_foo_sub2_resource" "res-1" {
  name     = "res1"
  location = "West Europe"
}
`,
				),
				configInfoWithDeps(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1/deep1/deep1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1/deep1/deep1",
					mustParseTFAddr("azurerm_foo_sub1_deep1_resource.res-2"),
					`
resource "azurerm_foo_sub1_deep1_resource" "res-2" {
  name   = "res2"
	foo_id = "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"
}
`,
					map[string]Dependency{},
					map[string][]Dependency{
						"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {
							{
								TFAddr:          mustParseTFAddr("azurerm_foo_sub1_resource.res-0"),
								AzureResourceId: "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1",
								TFResourceId:    "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
							},
							{
								TFAddr:          mustParseTFAddr("azurerm_foo_sub1_resource.res-1"),
								AzureResourceId: "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2",
								TFResourceId:    "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
							},
						},
					},
				),
			},
			expectedParentChildDeps: map[string]map[Dependency]bool{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1/deep1/deep1": {
					{
						TFAddr:          mustParseTFAddr("azurerm_foo_sub1_resource.res-0"),
						AzureResourceId: "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1",
						TFResourceId:    "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					}: true,
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.inputConfigs.populateParentChildDependency()
			for _, cfg := range testCase.inputConfigs {
				azureResourceId := cfg.AzureResourceID.String()
				expectedExplicitDeps := testCase.expectedParentChildDeps[azureResourceId]
				assert.Equal(t, expectedExplicitDeps, cfg.dependencies.parentChildDeps, "parentChildDeps matches expectation, azureResourceId: %s", azureResourceId)
			}
		})
	}
}
