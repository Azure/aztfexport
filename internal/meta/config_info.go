package meta

import (
	"github.com/hashicorp/hcl/v2/hclwrite"
	"io"
)

type ConfigInfos []ConfigInfo

type ConfigInfo struct {
	ImportItem
	hcl *hclwrite.File
}

func (cfg ConfigInfo) DumpHCL(w io.Writer) (int, error) {
	return w.Write(hclwrite.Format(cfg.hcl.Bytes()))
}

