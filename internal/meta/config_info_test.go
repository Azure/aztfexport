package meta

import (
	"fmt"
	"testing"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/magodo/armid"
	"github.com/stretchr/testify/assert"
)

func mustParseResourceId(id string) armid.ResourceId {
	parsed, err := armid.ParseResourceId(id)
	if err != nil {
		panic(err)
	}
	return parsed
}

func newConfigInfo(azid, tfid, tfaddr, hcl string, deps *Dependencies) ConfigInfo {
	cinfo := ConfigInfo{
		ImportItem: ImportItem{
			AzureResourceID: mustParseResourceId(azid),
			TFResourceId:    tfid,
			TFAddr:          mustParseTFAddr(tfaddr),
		},
		HCL: mustHclWriteParse(hcl),
		Dependencies: Dependencies{
			ByIdRef:          make(map[string]Dependency),
			ByIdRefAmbiguous: make(map[string][]Dependency),
		},
	}
	if deps != nil {
		cinfo.Dependencies = *deps
	}
	return cinfo
}

func TestConfigInfos_PopulateReferenceDeps(t *testing.T) {
	testCases := []struct {
		name                             string
		inputConfigs                     ConfigInfos
		expectedRgNameReferenceDeps      map[string]*Dependency             // key: AzureResourceId
		expectedIdReferenceDeps          map[string]map[string]Dependency   // key: AzureResourceId, inner key: TFResourceId
		expectedIdReferenceAmbiguousDeps map[string]map[string][]Dependency // key: AzureResourceId, inner key: TFResourceId
	}{
		{
			name: "no dependencies between resources",
			inputConfigs: []ConfigInfo{
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"azurerm_foo_resource.res-0",
					`
resource "azurerm_foo_resource" "res-0" {
  name              = "foo1"
}
`,
					nil,
				),
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					"azurerm_bar_resource.res-1",
					`
resource "azurerm_bar_resource" "res-1" {
  name              = "bar1"
}
`,
					nil,
				),
			},
			expectedIdReferenceDeps: map[string]map[string]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1": {},
			},
			expectedIdReferenceAmbiguousDeps: map[string]map[string][]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1": {},
			},
		},
		{
			name: "res-1 reference res-2 by id",
			inputConfigs: []ConfigInfo{
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1",
					"/subscriptions/123/resourceGroups/rg1",
					"azurerm_resource_group.res-0",
					`
resource "azurerm_resource_group" "res-0" {
  name     = "rg1"
  location = "West Europe"
}
`,
					nil,
				),
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"azurerm_foo_resource.res-1",
					`
resource "azurerm_foo_resource" "res-1" {
  name              = "foo1"
  resource_group_id = "/subscriptions/123/resourceGroups/rg1"
}
`,
					nil,
				),
			},
			expectedIdReferenceDeps: map[string]map[string]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {
					"/subscriptions/123/resourceGroups/rg1": {
						TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
						AzureResourceId: "/subscriptions/123/resourceGroups/rg1",
						TFResourceId:    "/subscriptions/123/resourceGroups/rg1",
					},
				},
				"/subscriptions/123/resourceGroups/rg1": make(map[string]Dependency),
			},
			expectedIdReferenceAmbiguousDeps: map[string]map[string][]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1":                                  {},
			},
		},
		{
			name: "res-0 and res-1 have different azure resource id, but same TF resource id, res-2 refer to this TF resource id: expect res-2 to have ambiguous id dep to the TF resource id",
			inputConfigs: []ConfigInfo{
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"azurerm_foo_resource.res-0",
					`resource "azurerm_foo_sub1_resource" "res-0" {
  name = "foo1_sub1"
}`,
					nil,
				),
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"azurerm_foo_resource.res-1",
					`
resource "azurerm_foo_sub2_resource" "res-1" {
  name = "foo1_sub2"
}
`,
					nil,
				),
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					"azurerm_bar_resource.res-2",
					`
resource "azurerm_bar_resource" "res-2" {
  name              = "bar1"
	foo_resource_id   = "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1"
}
`,
					nil,
				),
			},
			expectedIdReferenceDeps: map[string]map[string]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1":           {},
			},
			expectedIdReferenceAmbiguousDeps: map[string]map[string][]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2": {},
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1": {
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {
						{
							TFAddr:          mustParseTFAddr("azurerm_foo_resource.res-0"),
							AzureResourceId: "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub1/sub1",
							TFResourceId:    "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
						},
						{
							TFAddr:          mustParseTFAddr("azurerm_foo_resource.res-1"),
							AzureResourceId: "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1/sub2/sub2",
							TFResourceId:    "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
						},
					},
				},
			},
		},
		{
			name: "res-1 reference resource group by `resource_group_name`",
			inputConfigs: []ConfigInfo{
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1",
					"/subscriptions/123/resourceGroups/rg1",
					"azurerm_resource_group.res-0",
					`
resource "azurerm_resource_group" "res-0" {
  name     = "rg1"
  location = "West Europe"
}
`,
					nil,
				),
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"azurerm_foo_resource.res-1",
					`
resource "azurerm_foo_resource" "res-1" {
  name                = "foo1"
  resource_group_name = "rg1"
}
`,
					nil,
				),
			},
			expectedRgNameReferenceDeps: map[string]*Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {
					AzureResourceId: "/subscriptions/123/resourceGroups/rg1",
					TFResourceId:    "/subscriptions/123/resourceGroups/rg1",
					TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
				},
				"/subscriptions/123/resourceGroups/rg1": nil,
			},
			expectedIdReferenceDeps: map[string]map[string]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1":                                  {},
			},
			expectedIdReferenceAmbiguousDeps: map[string]map[string][]Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1":                                  {},
			},
		},
		{
			name: "res-1 reference resource group by `resource_group_name`, but in a different subscription",
			inputConfigs: []ConfigInfo{
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1",
					"/subscriptions/123/resourceGroups/rg1",
					"azurerm_resource_group.res-0",
					`
resource "azurerm_resource_group" "res-0" {
  name     = "rg1"
  location = "West Europe"
}
`,
					nil,
				),
				newConfigInfo(
					"/subscriptions/456/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"/subscriptions/456/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"azurerm_foo_resource.res-1",
					`
resource "azurerm_foo_resource" "res-1" {
  name                = "foo1"
  resource_group_name = "rg1"
}
`,
					nil,
				),
			},
			expectedRgNameReferenceDeps: map[string]*Dependency{
				"/subscriptions/456/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": nil,
				"/subscriptions/123/resourceGroups/rg1":                                  nil,
			},
			expectedIdReferenceDeps: map[string]map[string]Dependency{
				"/subscriptions/456/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1":                                  {},
			},
			expectedIdReferenceAmbiguousDeps: map[string]map[string][]Dependency{
				"/subscriptions/456/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {},
				"/subscriptions/123/resourceGroups/rg1":                                  {},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.inputConfigs.PopulateReferenceDeps()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for _, cfg := range testCase.inputConfigs {
				expectedIdRefDeps := testCase.expectedIdReferenceDeps[cfg.AzureResourceID.String()]
				assert.Equal(t, expectedIdRefDeps, cfg.Dependencies.ByIdRef, "idReferenceDeps matches expectation, azureResourceId: %s", cfg.AzureResourceID.String())

				expectedAmbiguousIdRefDeps := testCase.expectedIdReferenceAmbiguousDeps[cfg.AzureResourceID.String()]
				assert.Equal(t, expectedAmbiguousIdRefDeps, cfg.Dependencies.ByIdRefAmbiguous, "ambiguousIdReferenceDeps matches expectation, azureResourceId: %s", cfg.AzureResourceID.String())

				expectedRgNameRefDeps := testCase.expectedRgNameReferenceDeps[cfg.AzureResourceID.String()]
				assert.Equal(t, expectedRgNameRefDeps, cfg.Dependencies.ByRgNameRef, "nameReferenceDeps matches expectation, azureResourceId: %s", cfg.AzureResourceID.String())
			}
		})
	}
}

func TestConfigInfos_PopulateRelationDeps(t *testing.T) {
	testCases := []struct {
		name                string
		inputConfigs        ConfigInfos
		expectedRelationDep map[string]*Dependency // key: AzureResourceId
	}{
		{
			name: "no parent-child relationships",
			inputConfigs: []ConfigInfo{
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"azurerm_foo_resource.res-0",
					`
resource "azurerm_foo_resource" "res-0" {
  name = "foo1"
}
`,
					nil,
				),
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1",
					"azurerm_bar_resource.res-1",
					`
resource "azurerm_bar_resource" "res-1" {
  name = "bar1"
}
`,
					nil,
				),
			},
			expectedRelationDep: map[string]*Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": nil,
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Bar/bar/bar1": nil,
			},
		},
		{
			name: "res-0 is a parent of res-1: expect explicit dep from res-1 to res-0",
			inputConfigs: []ConfigInfo{
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1",
					"/subscriptions/123/resourceGroups/rg1",
					"azurerm_resource_group.res-0",
					`
resource "azurerm_resource_group" "res-0" {
  name     = "rg1"
  location = "West Europe"
}
`,
					nil,
				),
				newConfigInfo(
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1",
					"azurerm_foo_resource.res-1",
					`
resource "azurerm_foo_resource" "res-1" {
  name                = "foo1"
	resource_group_name = "rg1"
}
`,
					nil,
				),
			},
			expectedRelationDep: map[string]*Dependency{
				"/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foo/foo1": {
					TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
					AzureResourceId: "/subscriptions/123/resourceGroups/rg1",
					TFResourceId:    "/subscriptions/123/resourceGroups/rg1",
				},
				"/subscriptions/123/resourceGroups/rg1": nil,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.inputConfigs.PopulateRelationDeps()
			for _, cfg := range testCase.inputConfigs {
				azureResourceId := cfg.AzureResourceID.String()
				expectedExplicitDeps := testCase.expectedRelationDep[azureResourceId]
				assert.Equal(t, expectedExplicitDeps, cfg.Dependencies.ByRelation, "parentChildDeps matches expectation, azureResourceId: %s", azureResourceId)
			}
		})
	}
}

func TestConfigInfo_ApplyReferenceDepsToHCL(t *testing.T) {
	testCases := []struct {
		name            string
		inputHcl        string
		depsByIdRef     map[string]Dependency // key: TFResourceId
		depsByRgNameRef *Dependency
		expectedHcl     string
	}{
		{
			name: "no reference dependencies",
			inputHcl: `
resource azurerm_resource_foo self {
  name = "test"
  resource_group_name = "test"
}
`,
			depsByIdRef: make(map[string]Dependency),
			expectedHcl: `
resource azurerm_resource_foo self {
  name = "test"
  resource_group_name = "test"
}
`,
		},
		{
			name: "single reference dependency in top level attribute",
			inputHcl: `
resource azurerm_resource_bar self {
  name = "test"
  foo_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"
}
`,
			depsByIdRef: map[string]Dependency{
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123": {
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					TFAddr:          mustParseTFAddr("azurerm_foo_resource.res-1"),
				},
			},
			expectedHcl: `
resource azurerm_resource_bar self {
  name = "test"
  foo_id = azurerm_foo_resource.res-1.id
}
`,
		},
		{
			name: "single rg name dependency in top level attribute",
			inputHcl: `
resource azurerm_resource_foo self {
  name                = "test"
  resource_group_name = "rg1"
}
`,
			depsByRgNameRef: &Dependency{
				TFResourceId:    "/subscriptions/123/resourceGroups/rg1",
				AzureResourceId: "/subscriptions/123/resourceGroups/rg1",
				TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
			},
			expectedHcl: `
resource azurerm_resource_foo self {
  name                = "test"
  resource_group_name =azurerm_resource_group.res-0.name
}
`,
		},
		{
			name: "Mixed rg name dependency and id dependencies",
			inputHcl: `
resource azurerm_resource_foo self {
  name                = "test"
  resource_group_name = "rg1"
  bar_id              = "/subscriptions/123/providers/Microsoft.Bar/bars/bar1"
}
`,
			depsByIdRef: map[string]Dependency{
				"/subscriptions/123/providers/Microsoft.Bar/bars/bar1": {
					TFResourceId:    "/subscriptions/123/providers/Microsoft.Bar/bars/bar1",
					AzureResourceId: "/subscriptions/123/providers/Microsoft.Bar/bars/bar1",
					TFAddr:          mustParseTFAddr("azurerm_resource_bar.res-0"),
				},
			},
			depsByRgNameRef: &Dependency{
				TFResourceId:    "/subscriptions/123/resourceGroups/rg1",
				AzureResourceId: "/subscriptions/123/resourceGroups/rg1",
				TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
			},
			expectedHcl: `
resource azurerm_resource_foo self {
  name                = "test"
  resource_group_name =azurerm_resource_group.res-0.name
  bar_id              = azurerm_resource_bar.res-0.id
}
`,
		},
		{
			name: "multiple reference dependency in top level and nested block",
			inputHcl: `
resource azurerm_resource_foo self {
  name = "test"
  foo_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"

  some_block {
    bar_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456"
  }
}
`,
			depsByIdRef: map[string]Dependency{
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123": {
					TFAddr:          mustParseTFAddr("azurerm_foo_resource.res-1"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
				},
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456": {
					TFAddr:          mustParseTFAddr("azurerm_bar_resource.res-2"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
				},
			},
			expectedHcl: `
resource azurerm_resource_foo self {
  name = "test"
  foo_id = azurerm_foo_resource.res-1.id

  some_block {
    bar_id = azurerm_bar_resource.res-2.id
  }
}
`,
		},
		{
			name: "multiple reference dependency in array and maps",
			inputHcl: `
resource azurerm_resource_foo self {
  name = "test"
  foo_ids = ["/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"]

  bar_ids_map = {
    bar_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456"
  }
}
`,
			depsByIdRef: map[string]Dependency{
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123": {
					TFAddr:          mustParseTFAddr("azurerm_foo_resource.res-1"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
				},
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456": {
					TFAddr:          mustParseTFAddr("azurerm_bar_resource.res-2"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
				},
			},
			expectedHcl: `
resource azurerm_resource_foo self {
  name = "test"
  foo_ids = [azurerm_foo_resource.res-1.id]

  bar_ids_map = {
    bar_id = azurerm_bar_resource.res-2.id
  }
}
`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cinfo := ConfigInfo{
				Dependencies: Dependencies{
					ByIdRef:     testCase.depsByIdRef,
					ByRgNameRef: testCase.depsByRgNameRef,
				},
				HCL: mustHclWriteParse(testCase.inputHcl),
			}
			cinfo.applyRefDepsToHCL()
			assert.Equal(t, testCase.expectedHcl, string(cinfo.HCL.BuildTokens(nil).Bytes()))
		})
	}
}

func TestConfigInfo_ApplyExplicitDepsToHCL(t *testing.T) {
	testCases := []struct {
		name                 string
		inputHcl             string
		depsByByRelation     *Dependency
		depsByIdRefAmbiguous map[string][]Dependency // key: TFResourceId
		depsByIdRef          map[string]Dependency
		depsByRgNameRef      *Dependency
		expectedHcl          string
	}{
		{
			name: "no explicit or ambiguous dependencies",
			inputHcl: `
resource azurerm_resource_foo self {
  name = "test"
  foo_id = azurerm_foo_resource.res-1.id
}
`,
			expectedHcl: `
resource azurerm_resource_foo self {
  name = "test"
  foo_id = azurerm_foo_resource.res-1.id
}
`,
		},
		{
			name: "single parent child dependency",
			inputHcl: `
resource azurerm_resource_foo self {
  name = "test"
  resource_group_name = "test"
}
`,
			depsByByRelation: &Dependency{
				TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
				AzureResourceId: "/subscriptions/123/resourceGroups/123",
				TFResourceId:    "/subscriptions/123/resourceGroups/123",
			},
			expectedHcl: `
resource azurerm_resource_foo self {
  name = "test"
  resource_group_name = "test"
depends_on= [
azurerm_resource_group.res-0,
]
}
`,
		},
		{
			name: "single parent child dependency, but is covered by an id reference dependency",
			inputHcl: `
resource azurerm_resource_foo self {
  name = "test"
  resource_group_name = "test"
}
`,
			depsByByRelation: &Dependency{
				TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
				AzureResourceId: "/subscriptions/123/resourceGroups/123",
				TFResourceId:    "/subscriptions/123/resourceGroups/123",
			},
			depsByIdRef: map[string]Dependency{
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foos/foo1": {
					TFAddr:          mustParseTFAddr("azurerm_resource_foo.res-0"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foos/foo1",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foos/foo1",
				},
			},
			expectedHcl: `
resource azurerm_resource_foo self {
  name = "test"
  resource_group_name = "test"
}
`,
		},
		{
			name: "single parent child dependency, but is covered by a resource group name reference dependency",
			inputHcl: `
resource azurerm_resource_foo self {
  name = "test"
  resource_group_name = "test"
}
`,
			depsByByRelation: &Dependency{
				TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
				AzureResourceId: "/subscriptions/123/resourceGroups/test",
				TFResourceId:    "/subscriptions/123/resourceGroups/test",
			},
			depsByRgNameRef: &Dependency{
				TFAddr:          mustParseTFAddr("azurerm_resource_group.res-0"),
				AzureResourceId: "/subscriptions/123/resourceGroups/test",
				TFResourceId:    "/subscriptions/123/resourceGroups/test",
			},
			expectedHcl: `
resource azurerm_resource_foo self {
  name = "test"
  resource_group_name = "test"
}
`,
		},
		{
			name: "multiple ambiguous dependencies",
			inputHcl: `
resource azurerm_resource_foo self {
  name = "test"
  resource_group_name = "test"
}
`,
			depsByIdRefAmbiguous: map[string][]Dependency{
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123": {
					{
						TFAddr:          mustParseTFAddr("azurerm_foo_sub1_resource.res-1"),
						AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123/sub1/sub1",
						TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					},
					{
						TFAddr:          mustParseTFAddr("azurerm_foo_sub2_resource.res-2"),
						AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123/sub2/sub2",
						TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					},
				},
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456": {
					{
						TFAddr:          mustParseTFAddr("azurerm_bar_sub1_resource.res-3"),
						AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456/sub1/sub1",
						TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
					},
					{
						TFAddr:          mustParseTFAddr("azurerm_bar_sub2_resource.res-4"),
						AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456/sub2/sub2",
						TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
					},
				},
			},
			expectedHcl: `
resource azurerm_resource_foo self {
  name = "test"
  resource_group_name = "test"
depends_on= [
# One of azurerm_bar_sub1_resource.res-3,azurerm_bar_sub2_resource.res-4 (can't auto-resolve as their ids are identical)
# One of azurerm_foo_sub1_resource.res-1,azurerm_foo_sub2_resource.res-2 (can't auto-resolve as their ids are identical)
]
}
`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cinfo := ConfigInfo{
				Dependencies: Dependencies{
					ByRelation:       testCase.depsByByRelation,
					ByIdRef:          testCase.depsByIdRef,
					ByIdRefAmbiguous: testCase.depsByIdRefAmbiguous,
					ByRgNameRef:      testCase.depsByRgNameRef,
				},
				HCL: mustHclWriteParse(testCase.inputHcl),
			}

			assert.NoError(t, cinfo.applyExplicitDepsToHCL())
			actualHcl := string(cinfo.HCL.BuildTokens(nil).Bytes())
			assert.Equal(t, testCase.expectedHcl, actualHcl)
		})
	}
}

func mustHclWriteParse(input string) *hclwrite.File {
	file, diag := hclwrite.ParseConfig([]byte(input), "input.hcl", hcl.InitialPos)
	if diag.HasErrors() {
		panic(fmt.Sprintf("failed to parse HCL: %v", diag))
	}
	return file
}

func mustParseTFAddr(s string) tfaddr.TFAddr {
	tfAddr, err := tfaddr.ParseTFResourceAddr(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse TF resource address %s: %v", s, err))
	}
	return *tfAddr
}
