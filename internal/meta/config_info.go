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
	refDeps map[string]Dependency

	// Similar to refDeps, but due to multiple Azure resources can map to a same TF resource id, we can't decide which Azure resource
	// is depended on. Hence these will end up as comments inside "depends_on" block for the user to manually resolve.
	// The key is TFResourceId.
	ambiguousRefDeps map[string][]Dependency

	// Dependencies inferred via Azure resource id parent lookup, and will be applied in the "depends_on" block.
	// Especially, any dependency that is (transitively) present via refDepds will be filtered.
	parentChildDeps map[Dependency]bool
}

type Dependency struct {
	TFResourceId    string
	AzureResourceId string
	TFAddr          tfaddr.TFAddr
}
