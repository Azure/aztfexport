package meta

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/Azure/aztfexport/pkg/config"
	"github.com/magodo/armid"
)

type MetaGroupDummy struct {
	rg           string
	providerName string
}

func NewGroupMetaDummy(rg string, providerName string) *MetaGroupDummy {
	return &MetaGroupDummy{rg: rg, providerName: providerName}
}

func (m MetaGroupDummy) Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func (m MetaGroupDummy) ProviderName() string {
	return m.providerName
}

func (m MetaGroupDummy) Init(_ context.Context) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaGroupDummy) DeInit(_ context.Context) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaGroupDummy) ScopeName() string {
	return m.rg
}

func (m MetaGroupDummy) Workspace() string {
	return "example-workspace"
}

func (m MetaGroupDummy) ListResource(_ context.Context) (ImportList, error) {
	time.Sleep(500 * time.Millisecond)
	importList := make(ImportList, 0)
	ids := []string{
		"/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Network/virtualNetworks/example-network",
		"/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Compute/virtualMachines/example-machine",
		"/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Network/networkInterfaces/example-nic",
		"/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg/providers/Microsoft.Network/virtualNetworks/example-network/subnets/internal",
		"/subscriptions/0000000-0000-0000-0000-00000000000/resourceGroups/example-rg",
	}
	for _, id := range ids {
		azureResourceID, _ := armid.ParseResourceId(id)
		importList = append(importList, ImportItem{
			TFResourceId:    id,
			AzureResourceID: azureResourceID,
		})
	}
	return importList, nil
}

func (m MetaGroupDummy) CleanTFState(_ context.Context, _ string) {
	return
}

func (m MetaGroupDummy) ParallelImport(_ context.Context, items []*ImportItem) error {
	time.Sleep(time.Second)
	return nil
}

func (m MetaGroupDummy) PushState(_ context.Context) error {
	time.Sleep(time.Second)
	return nil
}

func (m MetaGroupDummy) GenerateCfg(_ context.Context, l ImportList) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaGroupDummy) ExportResourceMapping(_ context.Context, l ImportList) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaGroupDummy) ExportSkippedResources(_ context.Context, l ImportList) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m MetaGroupDummy) CleanUpWorkspace(_ context.Context) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (meta *MetaGroupDummy) SetPreImportHook(cb config.ImportCallback) {
}

func (meta *MetaGroupDummy) SetPostImportHook(cb config.ImportCallback) {
}
