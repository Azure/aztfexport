package meta

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
)

func TestApplyReferenceDependenciesToHcl(t *testing.T) {
	testCases := []struct {
		name        string
		inputHcl    string
		refDeps     map[string]Dependency // key: TFResourceId
		expectedHcl string
	}{
		{
			name: "no reference dependencies",
			inputHcl: `
  name = "test"
  resource_group_name = "test"
`,
			refDeps: make(map[string]Dependency),
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
			refDeps: map[string]Dependency{
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123": {
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					TFAddr:          tfAddr("azurerm_foo_resource.res-1"),
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
			refDeps: map[string]Dependency{
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123": {
					TFAddr:          tfAddr("azurerm_foo_resource.res-1"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
				},
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456": {
					TFAddr:          tfAddr("azurerm_bar_resource.res-2"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
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
			refDeps: map[string]Dependency{
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123": {
					TFAddr:          tfAddr("azurerm_foo_resource.res-1"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
				},
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456": {
					TFAddr:          tfAddr("azurerm_bar_resource.res-2"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
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
			applyReferenceDependenciesToHcl(body, testCase.refDeps)
			assert.Equal(t, testCase.expectedHcl, string(body.BuildTokens(nil).Bytes()))
		})
	}
}

func TestApplyParentChildAndAmbiguousDepsToHclBlock(t *testing.T) {
	testCases := []struct {
		name            string
		inputHcl        string
		parentChildDeps map[Dependency]bool
		ambiguousDeps   map[string][]Dependency // key: TFResourceId
		expectedHcl     string
	}{
		{
			name: "no explicit or ambiguous dependencies",
			inputHcl: `
  name = "test"
  foo_id = azurerm_foo_resource.res-1.id
`,
			parentChildDeps: make(map[Dependency]bool),
			ambiguousDeps:   make(map[string][]Dependency),
			expectedHcl: `
  name = "test"
  foo_id = azurerm_foo_resource.res-1.id
`,
		},
		{
			name: "single parent child dependency",
			inputHcl: `
  name = "test"
  resource_group_name = "test"
`,
			parentChildDeps: map[Dependency]bool{
				{
					TFAddr:          tfAddr("azurerm_resource_group.res-0"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.ResourceGroup/resourceGroup/123",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.ResourceGroup/resourceGroup/123",
				}: true,
			},
			ambiguousDeps: make(map[string][]Dependency),
			expectedHcl: `
  name = "test"
  resource_group_name = "test"
depends_on= [
azurerm_resource_group.res-0
]
`,
		},
		{
			name: "multiple parent child dependencies",
			inputHcl: `
  name = "test"
  resource_group_name = "test"
`,
			parentChildDeps: map[Dependency]bool{
				{
					TFAddr:          tfAddr("azurerm_resource_group.res-0"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.ResourceGroup/resourceGroup/123",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.ResourceGroup/resourceGroup/123",
				}: true,
				{
					TFAddr:          tfAddr("azurerm_resource_group.res-1"),
					AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.ResourceGroup/resourceGroup/124",
					TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.ResourceGroup/resourceGroup/124",
				}: true,
			},
			ambiguousDeps: make(map[string][]Dependency),
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
			parentChildDeps: make(map[Dependency]bool),
			ambiguousDeps: map[string][]Dependency{
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123": {
					{
						TFAddr:          tfAddr("azurerm_foo_sub1_resource.res-1"),
						AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123/sub1/sub1",
						TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					},
					{
						TFAddr:          tfAddr("azurerm_foo_sub2_resource.res-2"),
						AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123/sub2/sub2",
						TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					},
				},
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456": {
					{
						TFAddr:          tfAddr("azurerm_bar_sub1_resource.res-3"),
						AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456/sub1/sub1",
						TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
					},
					{
						TFAddr:          tfAddr("azurerm_bar_sub2_resource.res-4"),
						AzureResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456/sub2/sub2",
						TFResourceId:    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/456",
					},
				},
			},
			expectedHcl: `
  name = "test"
  resource_group_name = "test"
depends_on= [
# One of azurerm_bar_sub1_resource.res-3,azurerm_bar_sub2_resource.res-4 (can't auto-resolve as their ids are identical)
# One of azurerm_foo_sub1_resource.res-1,azurerm_foo_sub2_resource.res-2 (can't auto-resolve as their ids are identical)
]
`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := hclwriteBody(testCase.inputHcl)
			err := applyParentChildAndAmbiguousDepsToHclBlock(
				body,
				testCase.parentChildDeps,
				testCase.ambiguousDeps,
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
