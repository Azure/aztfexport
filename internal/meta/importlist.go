package meta

// TFResourceTypeSkip is a special resource type which represents to skip this resource from importing.
const TFResourceTypeSkip string = "Skip"

type ImportItem struct {
	// The azure resource id
	ResourceID string

	// Whether this azure resource failed to import into terraform (this might due to the TFResourceType doesn't match the resource)
	ImportError error

	// Whether this azure resource has been successfully imported
	Imported bool

	// Whether this azure resource failed to validate into terraform (tbh, this should reside in UI layer only)
	ValidateError error

	// The terraform resource type
	TFResourceType string

	// The terraform resource name
	TFResourceName string

	// Whether this TF resource type is from recommendation
	IsRecommended bool
}

func (item ImportItem) Skip() bool {
	return item.TFResourceType == TFResourceTypeSkip
}

func (item *ImportItem) TFAddr() string {
	if item.Skip() {
		return ""
	}
	return item.TFResourceType + "." + item.TFResourceName
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
