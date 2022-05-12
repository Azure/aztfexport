package meta

import (
	"time"
)

type MetaDummy struct {
	rg string
}

func newMetaDummy(rg string) (Meta, error) {
	return MetaDummy{rg: rg}, nil
}

func (m MetaDummy) Init() error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaDummy) ResourceGroupName() string {
	return m.rg
}

func (m MetaDummy) Workspace() string {
	return "example-workspace"
}

func (m MetaDummy) ListResource() (ImportList, error) {
	time.Sleep(500 * time.Millisecond)
	return ImportList{
		ImportItem{
			ResourceID: "/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Network/virtualNetworks/example-network",
		},
		ImportItem{
			ResourceID: "/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Compute/virtualMachines/example-machine",
		},
		ImportItem{
			ResourceID: "/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Network/networkInterfaces/example-nic",
		},
		ImportItem{
			ResourceID: "/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Network/virtualNetworks/example-network/subnets/internal",
		},
		ImportItem{
			ResourceID: "/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg",
		},
	}, nil
}

func (m MetaDummy) CleanTFState(_ string) {
	return
}

func (m MetaDummy) Import(item *ImportItem) {
	time.Sleep(time.Second)
	return
}

func (m MetaDummy) GenerateCfg(l ImportList) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaDummy) ExportResourceMapping(l ImportList) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}
