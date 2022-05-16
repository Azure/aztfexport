package resmap

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/aztfy/internal/tfaddr"
)

type ResourceMapping map[string]tfaddr.TFAddr

func (res ResourceMapping) MarshalJSON() ([]byte, error) {
	m := map[string]string{}
	for id, addr := range res {
		addr := addr.String()
		if addr == "" {
			addr = tfaddr.TFResourceTypeSkip
		}
		m[id] = addr
	}
	return json.Marshal(m)
}

func (res *ResourceMapping) UnmarshalJSON(b []byte) error {
	out := ResourceMapping{}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for id, addr := range m {
		var tfAddr tfaddr.TFAddr
		if addr == tfaddr.TFResourceTypeSkip {
			tfAddr.Type = tfaddr.TFResourceTypeSkip
		} else {
			pTFAddr, err := tfaddr.ParseTFResourceAddr(addr)
			if err != nil {
				return fmt.Errorf("parsing TF address %q: %v", addr, err)
			}
			tfAddr = *pTFAddr
		}
		out[id] = tfAddr
	}
	*res = out
	return nil
}
