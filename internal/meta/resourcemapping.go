package meta

import (
	"encoding/json"
	"fmt"
)

type ResourceMapping map[string]TFAddr

func (res ResourceMapping) MarshalJSON() ([]byte, error) {
	m := map[string]string{}
	for id, addr := range res {
		addr := addr.String()
		if addr == "" {
			addr = TFResourceTypeSkip
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
		var tfAddr TFAddr
		if addr == TFResourceTypeSkip {
			tfAddr.Type = TFResourceTypeSkip
		} else {
			pTFAddr, err := ParseTFResourceAddr(addr)
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
