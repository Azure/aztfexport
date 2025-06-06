package meta

import (
	"fmt"
	"testing"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/internal/tfresourceid"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
)

func TestApplyReferenceDependenciesToHcl(t *testing.T) {
	testCases := []struct {
		name                  string
		inputHcl              string
		referenceDependencies ReferenceDependencies
		expectedHcl           string
	}{
		{
			name: "no reference dependencies",
			inputHcl: `
  name = "test"
  resource_group_name = "test"
`,
			referenceDependencies: ReferenceDependencies{},
			expectedHcl: `
  name = "test"
  resource_group_name = "test"
`,
		},
		{
			name: "single reference dependency in top level attribute",
			inputHcl: `
  name = "test"
  foo_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"
`,
			referenceDependencies: ReferenceDependencies{
				internalMap: map[tfresourceid.TFResourceId]tfaddr.TFAddr{
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"): tfAddr("azurerm_foo_resource.res-1"),
				},
			},
			expectedHcl: `
  name = "test"
  foo_id = azurerm_foo_resource.res-1.id
`,
		},
		{
			name: "multiple reference dependency in top level and nested block",
			inputHcl: `
  name = "test"
  foo_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"

  some_block {
    bar_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456"
  }
`,
			referenceDependencies: ReferenceDependencies{
				internalMap: map[tfresourceid.TFResourceId]tfaddr.TFAddr{
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"): tfAddr("azurerm_foo_resource.res-1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456"): tfAddr("azurerm_bar_resource.res-2"),
				},
			},
			expectedHcl: `
  name = "test"
  foo_id = azurerm_foo_resource.res-1.id

  some_block {
    bar_id = azurerm_bar_resource.res-2.id
  }
`,
		},
		{
			name: "multiple reference dependency in array and maps",
			inputHcl: `
  name = "test"
  foo_ids = ["/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"]

  bar_ids_map = {
    bar_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456"
  }
`,
			referenceDependencies: ReferenceDependencies{
				internalMap: map[tfresourceid.TFResourceId]tfaddr.TFAddr{
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"): tfAddr("azurerm_foo_resource.res-1"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456"): tfAddr("azurerm_bar_resource.res-2"),
				},
			},
			expectedHcl: `
  name = "test"
  foo_ids = [azurerm_foo_resource.res-1.id]

  bar_ids_map = {
    bar_id = azurerm_bar_resource.res-2.id
  }
`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := hclwriteBody(testCase.inputHcl)
			applyReferenceDependenciesToHcl(body, &testCase.referenceDependencies)
			assert.Equal(t, testCase.expectedHcl, string(body.BuildTokens(nil).Bytes()))
		})
	}
}

func TestApplyExplicitAndAmbiguousDependenciesToHclBlock(t *testing.T) {
	testCases := []struct {
		name                  string
		inputHcl              string
		explicitDependencies  TFAddrSet
		ambiguousDependencies AmbiguousDependencies
		expectedHcl           string
	}{
		{
			name: "no explicit or ambiguous dependencies",
			inputHcl: `
  name = "test"
  foo_id = azurerm_foo_resource.res-1.id
`,
			explicitDependencies:  TFAddrSet{},
			ambiguousDependencies: AmbiguousDependencies{},
			expectedHcl: `
  name = "test"
  foo_id = azurerm_foo_resource.res-1.id
`,
		},
		{
			name: "single explicit dependency",
			inputHcl: `
  name = "test"
  resource_group_name = "test"
`,
			explicitDependencies:  *tfAddrSet("azurerm_resource_group.res-0"),
			ambiguousDependencies: AmbiguousDependencies{},
			expectedHcl: `
  name = "test"
  resource_group_name = "test"
depends_on= [
azurerm_resource_group.res-0
]
`,
		},
		{
			name: "multiple explicit dependencies",
			inputHcl: `
  name = "test"
  resource_group_name = "test"
`,
			explicitDependencies: *tfAddrSet(
				"azurerm_resource_group.res-0",
				"azurerm_resource_group.res-1",
			),
			ambiguousDependencies: AmbiguousDependencies{},
			expectedHcl: `
  name = "test"
  resource_group_name = "test"
depends_on= [
azurerm_resource_group.res-0,
azurerm_resource_group.res-1
]
`,
		},
		{
			name: "multiple ambiguous dependencies",
			inputHcl: `
  name = "test"
  resource_group_name = "test"
`,
			explicitDependencies: TFAddrSet{},
			ambiguousDependencies: AmbiguousDependencies{
				internalMap: map[tfresourceid.TFResourceId]*TFAddrSet{
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"): tfAddrSet("azurerm_foo_sub1_resource.res-1", "azurerm_foo_sub2_resource.res-2"),
					tfresourceid.TFResourceId("/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456"): tfAddrSet("azurerm_bar_sub1_resource.res-3", "azurerm_bar_sub2_resource.res-4"),
				},
			},
			expectedHcl: `
  name = "test"
  resource_group_name = "test"
depends_on= [
# One of azurerm_foo_sub1_resource.res-1,azurerm_foo_sub2_resource.res-2 (can't auto-resolve as their ids are identical)
# One of azurerm_bar_sub1_resource.res-3,azurerm_bar_sub2_resource.res-4 (can't auto-resolve as their ids are identical)
]
`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := hclwriteBody(testCase.inputHcl)
			err := applyExplicitAndAmbiguousDependenciesToHclBlock(
				body,
				&testCase.explicitDependencies,
				&testCase.ambiguousDependencies,
			)
			assert.NoError(t, err)

			actualHcl := string(body.BuildTokens(nil).Bytes())
			assert.Equal(t, testCase.expectedHcl, actualHcl)
		})
	}
}

func hclwriteBody(input string) *hclwrite.Body {
	file, diag := hclwrite.ParseConfig([]byte(input), "input.hcl", hcl.InitialPos)
	if diag.HasErrors() {
		panic(fmt.Sprintf("failed to parse HCL: %v", diag))
	}
	return file.Body()
}
