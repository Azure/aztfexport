package meta

import (
	"regexp"
	"testing"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/magodo/armid"
	"github.com/stretchr/testify/require"
)

func TestExcludeImportList(t *testing.T) {
	armidMustParse := func(id string) armid.ResourceId {
		out, err := armid.ParseResourceId(id)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	input := ImportList{
		{
			AzureResourceID: armidMustParse("/subscriptions/x/resourceGroups/foo"),
			TFAddr: tfaddr.TFAddr{
				Type: "azurerm_resource_group",
			},
		},
		{
			AzureResourceID: armidMustParse("/subscriptions/x/resourceGroups/bar"),
			TFAddr: tfaddr.TFAddr{
				Type: "azurerm_resource_group",
			},
		},
		{
			AzureResourceID: armidMustParse("/subscriptions/x/resourceGroups/foo/providers/Microsoft.Network/virtualNetworks/net1"),
			TFAddr: tfaddr.TFAddr{
				Type: "azurerm_virtual_network",
			},
		},
		{
			AzureResourceID: armidMustParse("/subscriptions/x/resourceGroups/foo/providers/Microsoft.Foo/foos/foo1"),
		},
	}
	cases := []struct {
		name                     string
		expectCount              int
		excludeAzureResource     []regexp.Regexp
		excludeTerraformResource []string
	}{
		{
			name:                 "Exclude one by azure id",
			expectCount:          len(input) - 1,
			excludeAzureResource: []regexp.Regexp{*regexp.MustCompile(`foo$`)},
		},
		{
			name:                     "Exclude one by tf type",
			expectCount:              len(input) - 1,
			excludeTerraformResource: []string{"azurerm_virtual_network"},
		},
		{
			name:                 "Exclude three by azure id",
			expectCount:          len(input) - 3,
			excludeAzureResource: []regexp.Regexp{*regexp.MustCompile(`resourceGroups/foo`)},
		},
		{
			name:                     "Exclude two by tf type",
			expectCount:              len(input) - 2,
			excludeTerraformResource: []string{"azurerm_resource_group"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			meta := baseMeta{
				excludeAzureResources:     tt.excludeAzureResource,
				excludeTerraformResources: tt.excludeTerraformResource,
			}
			output := meta.excludeImportList(input)
			require.Equal(t, tt.expectCount, len(output))
		})
	}
}
