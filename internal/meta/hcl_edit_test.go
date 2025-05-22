package meta

import (
	"testing"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/require"
)

func TestHclBlockAppendLifecycle(t *testing.T) {
	cases := []struct {
		name          string
		ignoreChanges []string
		expect        string
	}{
		{
			name:          "no lifecycle should be generated",
			ignoreChanges: nil,
			expect:        "",
		},
		{
			name:          "with ignore_changes",
			ignoreChanges: []string{"foo", "bar"},
			expect: `lifecycle {
  ignore_changes = [
    foo,
    bar,
  ]
}
`,
		},
	}

	for _, c := range cases {
		b := hclwrite.NewFile().Body()
		require.NoError(t, hclBlockAppendLifecycle(b, c.ignoreChanges), c.name)
		require.Equal(t, string(hclwrite.Format(b.BuildTokens(nil).Bytes())), c.expect, c.name)
	}
}

func TestReplaceIdValuedTokensWithTFAddr(t *testing.T) {
	cases := []struct {
		description     string
		depTfResourceId string
		depTfAddr       string
		inputHclBody    string
		expectedHclBody string
		expectedRetVal  bool
	}{
		{
			description:     "single id value should be replaced with tf addr",
			depTfResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
			depTfAddr:       "azurerm_foo_resource.example",
			inputHclBody: `
  name   = "test"
  foo_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"
`,
			expectedHclBody: `
  name   = "test"
  foo_id = azurerm_foo_resource.example.id
`,
			expectedRetVal: true,
		},
		{
			description:     "multiple id values should be replaced with tf addr",
			depTfResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
			depTfAddr:       "azurerm_foo_resource.example",
			inputHclBody: `
  name     = "test"
  foo_x_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"
  foo_y_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"
`,
			expectedHclBody: `
  name     = "test"
  foo_x_id = azurerm_foo_resource.example.id
  foo_y_id = azurerm_foo_resource.example.id
`,
			expectedRetVal: true,
		},
		{
			description:     "no replacement if no id value matches",
			depTfResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/123",
			depTfAddr:       "azurerm_bar_resource.example",
			inputHclBody: `
  name   = "test"
  foo_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"
`,
			expectedHclBody: `
  name   = "test"
  foo_id = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"
`,
			expectedRetVal: false,
		},
		{
			description:     "empty block",
			depTfResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/123",
			depTfAddr:       "azurerm_bar_resource.example",
			inputHclBody:    ``,
			expectedHclBody: ``,
			expectedRetVal:  false,
		},
		{
			description:     "id value in a list",
			depTfResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
			depTfAddr:       "azurerm_foo_resource.example",
			inputHclBody: `
  name    = "test"
  foo_ids = [ 
    "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
    "/this/should/not/be/changed",
  ]
`,
			expectedHclBody: `
  name    = "test"
  foo_ids = [ 
    azurerm_foo_resource.example.id,
    "/this/should/not/be/changed",
  ]
`,
			expectedRetVal: true,
		},
		{
			description:     "id value in a map",
			depTfResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
			depTfAddr:       "azurerm_foo_resource.example",
			inputHclBody: `
  name    = "test"
  foo_ids_map = {
    fst = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
    snd = "/this/should/not/be/changed",
  }
`,
			expectedHclBody: `
  name    = "test"
  foo_ids_map = {
    fst = azurerm_foo_resource.example.id,
    snd = "/this/should/not/be/changed",
  }
`,
			expectedRetVal: true,
		},
		{
			description:     "id value in a nested block",
			depTfResourceId: "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
			depTfAddr:       "azurerm_foo_resource.example",
			inputHclBody: `
  name    = "test"
  some_block {
    foo_ids = [
      "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"
    ]
  }
`,
			expectedHclBody: `
  name    = "test"
  some_block {
    foo_ids = [
      azurerm_foo_resource.example.id
    ]
  }
`,
			expectedRetVal: true,
		},
	}

	for _, c := range cases {
		cfg := configInfo(t, c.depTfResourceId, c.depTfAddr)
		body := hclwriteBody(t, c.inputHclBody)

		actualRetVal := replaceIdValuedTokensWithTFAddr(body, cfg)

		require.Equal(t, c.expectedRetVal, actualRetVal, "'%s': expectedRetVal should match actual", c.description)
		require.Equal(t, c.expectedHclBody, string(body.BuildTokens(nil).Bytes()), "'%s': expectedHclBody should match actual", c.description)
	}

}

func configInfo(t *testing.T, tfResourceId string, tfAddr string) ConfigInfo {
	tfResourceAddr, err := tfaddr.ParseTFResourceAddr(tfAddr)
	if err != nil {
		t.Fatalf("failed to parse tfAddr: %v", err)
	}

	return ConfigInfo{
		ImportItem: ImportItem{
			TFAddr:       *tfResourceAddr,
			TFResourceId: tfResourceId,
		},
	}
}

func hclwriteBody(t *testing.T, input string) *hclwrite.Body {
	file, diag := hclwrite.ParseConfig([]byte(input), "input.hcl", hcl.InitialPos)
	if diag.HasErrors() {
		t.Fatalf("failed to parse HCL: %v", diag)
	}
	return file.Body()
}

func TestHclBlockUpdateDependency(t *testing.T) {
	cases := []struct {
		description     string
		deps            []Dependency
		cfgset          map[string]ConfigInfo
		inputHclBody    string
		expectedHclBody string
	}{
		{
			description: "foo_id should be replaced with TF address, no depends_on is added",
			deps: []Dependency{
				{[]string{"/subscriptions/123/resourceGroups/123"}},
				{[]string{"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"}},
			},
			cfgset: map[string]ConfigInfo{
				"/subscriptions/123/resourceGroups/123": configInfo(
					t,
					"/subscriptions/123/resourceGroups/123",
					"azurerm_resource_group.res-0",
				),
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123": configInfo(
					t,
					"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					"azurerm_foo_resource.res-1",
				),
			},
			inputHclBody: `
name                = "test"
resource_group_name = "test"
foo_id              = "/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"
`,
			expectedHclBody: `
name                = "test"
resource_group_name = "test"
foo_id              = azurerm_foo_resource.res-1.id
`,
		},
		{
			description: `depends on foo by foo_name, 
			depends on resource group by resource_group_id, 
			foo also depends on the same resource group: 
			expected resource_group_id to be replaced with TF address and there is a depends_on on foo`,
			deps: []Dependency{
				{[]string{"/subscriptions/123/resourceGroups/123"}},
				{[]string{"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123"}},
			},
			cfgset: map[string]ConfigInfo{
				"/subscriptions/123/resourceGroups/123": configInfo(
					t,
					"/subscriptions/123/resourceGroups/123",
					"azurerm_resource_group.res-0",
				),
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123": configInfo(
					t,
					"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					"azurerm_foo_resource.res-1",
				),
			},
			inputHclBody: `
name              = "test"
resource_group_id = "/subscriptions/123/resourceGroups/123"
foo_name          = "test"
`,
			expectedHclBody: `
name              = "test"
resource_group_id = azurerm_resource_group.res-0.id
foo_name          = "test"
depends_on = [
  azurerm_foo_resource.res-1
]
`,
		},
		{
			description: "ambiguous dependency explicitly specified via depends_on",
			deps: []Dependency{
				{[]string{"/subscriptions/123/resourceGroups/123"}},
				{[]string{
					"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123/subfoo/1",
					"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123/subfoo/2",
				}},
			},
			cfgset: map[string]ConfigInfo{
				"/subscriptions/123/resourceGroups/123": configInfo(
					t,
					"/subscriptions/123/resourceGroups/123",
					"azurerm_resource_group.res-0",
				),
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123/subfoo/1": configInfo(
					t,
					"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					"azurerm_subfoo_resource.res-1",
				),
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123/subfoo/2": configInfo(
					t,
					"/subscriptions/123/resourceGroups/123/providers/Microsoft.Foo/foo/123",
					"azurerm_subfoo_resource.res-2",
				),
			},
			inputHclBody: `
name                = "test"
resource_group_name = "test"
`,
			expectedHclBody: `
name                = "test"
resource_group_name = "test"
depends_on = [
  # One of azurerm_subfoo_resource.res-1,azurerm_subfoo_resource.res-2 (can't auto-resolve as their ids are identical)
  azurerm_resource_group.res-0
]
`,
		},
	}

	for _, c := range cases {
		body := hclwriteBody(t, c.inputHclBody)
		require.NoError(t, hclBlockUpdateDependency(body, c.deps, c.cfgset), c.description)
		require.Equal(t, string(hclwrite.Format(body.BuildTokens(nil).Bytes())), c.expectedHclBody, c.description)
	}
}

func TestDescendantResourceIdInCollection(t *testing.T) {
	cases := []struct {
		description     string
		resourceId      string
		resourceIdsList []string
		expectedRetVal  bool
	}{
		{
			description: "descendant resource id not in collection",
			resourceId:  "/subscriptions/123/resourceGroups/123",
			resourceIdsList: []string{
				"/subscriptions/123/resourceGroups/456/providers/Microsoft.Bar/bar/123",
				"/subscriptions/123/resourceGroups/456/providers/Microsoft.Bar/bar/123/subbar/456",
			},
			expectedRetVal: false,
		},
		{
			description: "descendant resource id in collection",
			resourceId:  "/subscriptions/123/resourceGroups/123",
			resourceIdsList: []string{
				"/subscriptions/123/resourceGroups/456/providers/Microsoft.Foo/foo/aaa",
				"/subscriptions/123/resourceGroups/123/providers/Microsoft.Bar/bar/bbb",
			},
			expectedRetVal: true,
		},
	}

	for _, c := range cases {
		actualRetVal := descendantResourceIdInCollection(c.resourceId, &c.resourceIdsList)
		require.Equal(t, c.expectedRetVal, actualRetVal, "'%s': expectedRetVal should match actual", c.description)
	}
}
