package meta

import (
	"github.com/Azure/aztfy/internal/tfaddr"
	"github.com/magodo/armid"
)

type ImportItem struct {
	// Azure resource Id
	AzureResourceID armid.ResourceId

	// The TF resource id
	TFResourceId string

	// Whether this azure resource failed to import into terraform (this might due to the TFResourceType doesn't match the resource)
	ImportError error

	// Whether this azure resource has been successfully imported
	Imported bool

	// Whether this azure resource failed to validate into terraform (tbh, this should reside in UI layer only)
	ValidateError error

	// The terraform resource
	TFAddr tfaddr.TFAddr

	// Whether this TF resource type is from recommendation
	IsRecommended bool

	Recommendations []string
}

func (item ImportItem) Skip() bool {
	return item.TFAddr.Type == ""
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
