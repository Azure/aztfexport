package meta

import (
	"time"
)

var _ Meta = &MetaGroupDummy{}

type MetaGroupDummy struct {
	rg string
}

func newGroupMetaDummy(rg string) (Meta, error) {
	return MetaGroupDummy{rg: rg}, nil
}

func (m MetaGroupDummy) Init() error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaGroupDummy) ScopeName() string {
	return m.rg
}

func (m MetaGroupDummy) Workspace() string {
	return "example-workspace"
}

func (m MetaGroupDummy) ListResource() (ImportList, error) {
	time.Sleep(500 * time.Millisecond)
	return ImportList{
		ImportItem{
			TFResourceId: "/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Network/virtualNetworks/example-network",
		},
		ImportItem{
			TFResourceId: "/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Compute/virtualMachines/example-machine",
		},
		ImportItem{
			TFResourceId: "/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Network/networkInterfaces/example-nic",
		},
		ImportItem{
			TFResourceId: "/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Network/virtualNetworks/example-network/subnets/internal",
		},
		ImportItem{
			TFResourceId: "/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg",
		},
	}, nil
}

func (m MetaGroupDummy) CleanTFState(_ string) {
	return
}

func (m MetaGroupDummy) Import(item *ImportItem) {
	time.Sleep(time.Second)
	return
}

func (m MetaGroupDummy) MultipleImport(items []*ImportItem) {
	time.Sleep(time.Second)
	return
}

func (m MetaGroupDummy) PushState() error {
	time.Sleep(time.Second)
	return nil
}

func (m MetaGroupDummy) GenerateCfg(l ImportList) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaGroupDummy) ExportResourceMapping(l ImportList) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaGroupDummy) ExportSkippedResources(l ImportList) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaGroupDummy) CleanUpWorkspace() error {
	time.Sleep(500 * time.Millisecond)
	return nil
}
