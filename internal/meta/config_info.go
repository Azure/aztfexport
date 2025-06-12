package meta

import (
	"io"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

type ConfigInfos []ConfigInfo

type ConfigInfo struct {
	ImportItem

	dependencies Dependencies

	hcl *hclwrite.File
}

func (cfg ConfigInfo) DumpHCL(w io.Writer) (int, error) {
	out := hclwrite.Format(cfg.hcl.Bytes())
	return w.Write(out)
}

type Dependencies struct {
	// Dependencies inferred by scanning for resource id values, will be applied by substituting with TF address
	// Key is TFResourceId
	referenceDeps map[string]Dependency

	// Dependencies inferred via resource id parent lookup. If not yet (transitively) present
	// in referenceDependencies, will be applied as depends_on meta argument
	parentChildDeps map[Dependency]bool

	// Multiple TF address for a TF resource id can exist, these will be appended as a comment inside depends_on block for
	// user to manually resolve. The key is TFResourceId.
	ambiguousDeps map[string][]Dependency
}

type Dependency struct {
	TFResourceId    string
	AzureResourceId string
	TFAddr          tfaddr.TFAddr
}
