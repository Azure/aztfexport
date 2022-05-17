package armtemplate_test

import (
	"encoding/json"
	"testing"

	"github.com/Azure/aztfy/internal/armtemplate"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalTemplate(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		expect armtemplate.Template
	}{
		{
			name: "one level dependency",
			input: `
{
	"resources": [
		{
			"type": "Microsoft.Storage/storageAccounts",
			"name": "a"
		},
		{
			"type": "Microsoft.Storage/storageAccounts/fileServices",
			"name": "a/default",
			"dependsOn": [
				"[resourceId('Microsoft.Storage/storageAccounts', 'a')]"
			]
		}
	]
}
`,
			expect: armtemplate.Template{
				Resources: []armtemplate.Resource{
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Storage/storageAccounts",
							Name: "a",
						},
						DependsOn: nil,
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Storage/storageAccounts/fileServices",
							Name: "a/default",
						},
						DependsOn: armtemplate.ResourceIds{
							{
								Type: "Microsoft.Storage/storageAccounts",
								Name: "a",
							},
						},
					},
				},
			},
		},
		{
			name: "multi-level dependency",
			input: `
{
	"resources": [
		{
			"type": "Microsoft.Network/networkInterfaces",
			"name": "nic",
			"dependsOn": [
				"[resourceId('Microsoft.Network/publicIPAddresses', 'pip')]",
				"[resourceId('Microsoft.Network/virtualNetworks/subnets', 'vnet', 'subnet')]",
				"[resourceId('Microsoft.Network/networkSecurityGroups', 'nsg')]"
			]
		},
		{
			"type": "Microsoft.Network/virtualNetworks/subnets",
			"name": "vnet/subnet",
			"dependsOn": [
				"[resourceId('Microsoft.Network/virtualNetworks', 'vnet')]",
				"[resourceId('Microsoft.Network/networkSecurityGroups', 'nsg')]"
			]
		},
		{
			"type": "Microsoft.Network/networkSecurityGroups/securityRules",
			"name": "nsg/nsr",
			"dependsOn": [
				"[resourceId('Microsoft.Network/networkSecurityGroups', 'nsg')]"
			]
 		},
		{
			"type": "Microsoft.Network/networkSecurityGroups",
			"name": "nsg"
		},
		{
			"type": "Microsoft.Network/virtualNetworks",
			"name": "vnet"
		},
		{
			"type": "Microsoft.Network/publicIPAddresses",
			"name": "pip"
		}
	]
}
`,
			expect: armtemplate.Template{
				Resources: []armtemplate.Resource{
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/networkInterfaces",
							Name: "nic",
						},
						DependsOn: armtemplate.ResourceIds{
							{
								Type: "Microsoft.Network/publicIPAddresses",
								Name: "pip",
							},
							{
								Type: "Microsoft.Network/virtualNetworks/subnets",
								Name: "vnet/subnet",
							},
							{
								Type: "Microsoft.Network/networkSecurityGroups",
								Name: "nsg",
							},
						},
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/virtualNetworks/subnets",
							Name: "vnet/subnet",
						},
						DependsOn: armtemplate.ResourceIds{
							{
								Type: "Microsoft.Network/virtualNetworks",
								Name: "vnet",
							},
							{
								Type: "Microsoft.Network/networkSecurityGroups",
								Name: "nsg",
							},
						},
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/networkSecurityGroups/securityRules",
							Name: "nsg/nsr",
						},
						DependsOn: armtemplate.ResourceIds{
							{
								Type: "Microsoft.Network/networkSecurityGroups",
								Name: "nsg",
							},
						},
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/networkSecurityGroups",
							Name: "nsg",
						},
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/virtualNetworks",
							Name: "vnet",
						},
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/publicIPAddresses",
							Name: "pip",
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		var out armtemplate.Template
		require.NoError(t, json.Unmarshal([]byte(c.input), &out), c.name)
		require.Equal(t, c.expect, out, c.name)
	}
}

func TestDependencyInfo(t *testing.T) {
	cases := []struct {
		name   string
		subId  string
		rg     string
		input  armtemplate.Template
		expect map[string][]string
	}{
		{
			name:  "multiple-level dependency",
			subId: "sub1",
			rg:    "rg1",
			input: armtemplate.Template{
				Resources: []armtemplate.Resource{
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/networkInterfaces",
							Name: "nic",
						},
						DependsOn: armtemplate.ResourceIds{
							{
								Type: "Microsoft.Network/publicIPAddresses",
								Name: "pip",
							},
							{
								Type: "Microsoft.Network/virtualNetworks/subnets",
								Name: "vnet/subnet",
							},
							{
								Type: "Microsoft.Network/networkSecurityGroups",
								Name: "nsg",
							},
						},
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/virtualNetworks/subnets",
							Name: "vnet/subnet",
						},
						DependsOn: armtemplate.ResourceIds{
							{
								Type: "Microsoft.Network/virtualNetworks",
								Name: "vnet",
							},
							{
								Type: "Microsoft.Network/networkSecurityGroups",
								Name: "nsg",
							},
						},
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/networkSecurityGroups/securityRules",
							Name: "nsg/nsr",
						},
						DependsOn: armtemplate.ResourceIds{
							{
								Type: "Microsoft.Network/networkSecurityGroups",
								Name: "nsg",
							},
						},
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/networkSecurityGroups",
							Name: "nsg",
						},
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/virtualNetworks",
							Name: "vnet",
						},
					},
					{
						ResourceId: armtemplate.ResourceId{
							Type: "Microsoft.Network/publicIPAddresses",
							Name: "pip",
						},
					},
				},
			},
			expect: map[string][]string{
				"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/networkInterfaces/nic": {
					"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/publicIPAddresses/pip",
					"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet",
					"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/networkSecurityGroups/nsg",
				},
				"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet": {
					"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet",
					"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/networkSecurityGroups/nsg",
				},
				"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/networkSecurityGroups/nsg/securityRules/nsr": {
					"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/networkSecurityGroups/nsg",
				},
				"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/networkSecurityGroups/nsg": {
					"/subscriptions/sub1/resourceGroups/rg1",
				},
				"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet": {
					"/subscriptions/sub1/resourceGroups/rg1",
				},
				"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/publicIPAddresses/pip": {
					"/subscriptions/sub1/resourceGroups/rg1",
				},
			},
		},
	}

	for _, c := range cases {
		require.Equal(t, c.expect, c.input.Qualify(c.subId, c.rg).DependencyInfo(), c.name)
	}
}

func TestNewResourceIdFromCallExpr(t *testing.T) {
	cases := []struct {
		name   string
		expr   string
		expect armtemplate.ResourceId
		error  bool
	}{
		{
			name:  "empty",
			expr:  "",
			error: true,
		},
		{
			name:  "no args",
			expr:  "[resourceId()]",
			error: true,
		},
		{
			name: "one level",
			expr: "[resourceId('Microsoft.Storage/storageAccounts', 'a')]",
			expect: armtemplate.ResourceId{
				Type: "Microsoft.Storage/storageAccounts",
				Name: "a",
			},
			error: false,
		},
		{
			name: "two levels",
			expr: "[resourceId('Microsoft.Storage/storageAccounts/services', 'a', 'b')]",
			expect: armtemplate.ResourceId{
				Type: "Microsoft.Storage/storageAccounts/services",
				Name: "a/b",
			},
			error: false,
		},
	}

	for _, c := range cases {
		output, err := armtemplate.NewResourceIdFromCallExpr(c.expr)
		if c.error {
			require.Error(t, err, c.name)
			continue
		}
		require.NoError(t, err, c.name)
		require.Equal(t, c.expect, *output, c.name)
	}
}
