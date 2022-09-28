package meta

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/aztfy/internal/config"
)

const ResourceMappingFileName = "aztfyResourceMapping.json"
const SkippedResourcesFileName = "aztfySkippedResources.txt"

type GroupMeta interface {
	meta
	ScopeName() string
	ListResource() (ImportList, error)
	ExportSkippedResources(l ImportList) error
}

func NewGroupMeta(cfg config.GroupConfig) (GroupMeta, error) {
	if cfg.MockClient {
		return newGroupMetaDummy(cfg.ResourceGroupName)
	}

	switch {
	case cfg.ResourceGroupName != "":
		return newMetaResourceGroup(cfg)
	case cfg.ARGPredicate != "":
		return newMetaQuery(cfg)
	case cfg.MappingFile != "":
		return newMetaMap(cfg)
	default:
		return nil, fmt.Errorf("invalid group config")
	}
}

func resourceNamePattern(p string) (prefix, suffix string) {
	if pos := strings.LastIndex(p, "*"); pos != -1 {
		return p[:pos], p[pos+1:]
	}
	return p, ""
}

func ptr[T any](v T) *T {
	return &v
}

func exportSkippedResources(l ImportList, wsp string) error {
	var sl []string
	for _, item := range l {
		if item.Skip() {
			sl = append(sl, "- "+item.AzureResourceID.String())
		}
	}
	if len(sl) == 0 {
		return nil
	}

	output := filepath.Join(wsp, SkippedResourcesFileName)
	if err := os.WriteFile(output, []byte(fmt.Sprintf(`Following resources are marked to be skipped:

%s

They are either not Terraform candidate resources (e.g. not managed by users), or not supported by the Terraform AzureRM provider yet.
`, strings.Join(sl, "\n"))), 0644); err != nil {
		return fmt.Errorf("writing the skipped resources to %s: %v", output, err)
	}
	return nil
}
