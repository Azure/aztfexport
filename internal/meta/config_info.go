package meta

import (
	"bytes"
	"io"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

type ConfigInfos []ConfigInfo

type ConfigInfo struct {
	ImportItem
	hcl *hclwrite.File
}

func (cfg ConfigInfo) DumpHCL(w io.Writer) (int, error) {
	out := hclwrite.Format(cfg.hcl.Bytes())
	// Hack: removing the leading warning comments before each resource config,
	// which is generated via "terraform add".
	out = out[bytes.Index(out, []byte(`resource "`)):]
	return w.Write(out)
}
