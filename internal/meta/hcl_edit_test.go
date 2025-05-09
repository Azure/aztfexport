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
