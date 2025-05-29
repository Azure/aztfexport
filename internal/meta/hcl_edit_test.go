package meta

import (
	"testing"

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
