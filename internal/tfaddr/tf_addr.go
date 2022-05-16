package tfaddr

import (
	"fmt"
	"strings"
)

// TFResourceTypeSkip is a special resource type which represents to skip this resource from importing.
const TFResourceTypeSkip string = "Skip"

type TFAddr struct {
	Type string
	Name string
}

func (res TFAddr) Skip() bool {
	return res.Type == TFResourceTypeSkip
}

func (res TFAddr) String() string {
	if res.Skip() {
		return ""
	}
	return res.Type + "." + res.Name
}

func ParseTFResourceAddr(v string) (*TFAddr, error) {
	segs := strings.Split(v, ".")
	if len(segs) != 2 || segs[0] == "" || segs[1] == "" {
		return nil, fmt.Errorf("malformed resource address: %s", v)
	}
	return &TFAddr{Type: segs[0], Name: segs[1]}, nil
}
