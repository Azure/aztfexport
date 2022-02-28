package meta

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

type ImportItem struct {
	// The azure resource id
	ResourceID string

	// Whether this azure resource failed to import into terraform (this might due to the TFResourceType doesn't match the resource)
	ImportError error

	// Whether this azure resource has been successfully imported
	Imported bool

	// Whether this azure resource failed to validate into terraform (tbh, this should reside in UI layer only)
	ValidateError error

	// The terraform resource
	TFAddr TFAddr

	// Whether this TF resource type is from recommendation
	IsRecommended bool
}

func (item ImportItem) Skip() bool {
	return item.TFAddr.Skip()
}

type ImportList []ImportItem

func (l ImportList) NonSkipped() ImportList {
	var out ImportList
	for _, item := range l {
		if item.Skip() {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (l ImportList) ImportErrored() ImportList {
	var out ImportList
	for _, item := range l {
		if item.ImportError == nil {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (l ImportList) Imported() ImportList {
	var out ImportList
	for _, item := range l {
		if item.Imported {
			out = append(out, item)
		}
	}
	return out
}
