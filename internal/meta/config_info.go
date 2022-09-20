package meta

import (
	"io"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

type ConfigInfos []ConfigInfo

type ConfigInfo struct {
	ImportItem
	DependsOn []string
	hcl       *hclwrite.File
}

func (cfg ConfigInfo) DumpHCL(w io.Writer) (int, error) {
	out := hclwrite.Format(cfg.hcl.Bytes())
	return w.Write(out)
}
