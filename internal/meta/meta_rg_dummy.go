package meta

import (
	"time"
)

var _ RgMeta = &MetaRgDummy{}

type MetaRgDummy struct {
	rg string
}

func newRgMetaDummy(rg string) (RgMeta, error) {
	return MetaRgDummy{rg: rg}, nil
}

func (m MetaRgDummy) Init() error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaRgDummy) ResourceGroupName() string {
	return m.rg
}

func (m MetaRgDummy) Workspace() string {
	return "example-workspace"
}

func (m MetaRgDummy) ListResource() (ImportList, error) {
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

func (m MetaRgDummy) CleanTFState(_ string) {
	return
}

func (m MetaRgDummy) Import(item *ImportItem) {
	time.Sleep(time.Second)
	return
}

func (m MetaRgDummy) GenerateCfg(l ImportList) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaRgDummy) ExportResourceMapping(l ImportList) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}
