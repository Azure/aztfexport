package meta

import (
	"io"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

type ConfigInfos []ConfigInfo

type ConfigInfo struct {
	ImportItem

	// Dependencies inferred by scanning for resource id values, will be applied by substituting with TF address
	referenceDependencies ReferenceDependencies

	// Dependencies inferred via resource id parent lookup. If not yet (transitively) present
	// in referenceDependencies, will be applied as depends_on meta argument
	explicitDependencies TFAddrSet

	// Multiple TF address for a TF resource id can exist, these will be appended as a comment inside depends_on block for
	// user to manually resolve
	ambiguousDependencies AmbiguousDependencies

	hcl *hclwrite.File
}

func (cfg ConfigInfo) DumpHCL(w io.Writer) (int, error) {
	out := hclwrite.Format(cfg.hcl.Bytes())
	return w.Write(out)
}
