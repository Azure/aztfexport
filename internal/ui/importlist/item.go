package importlist

import (
	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/internal/ui/common"
	"github.com/magodo/textinput"
)

type Item struct {
	idx       int
	v         meta.ImportItem
	textinput textinput.Model
}

func (i Item) Title() string {
	switch {
	case i.v.ValidateError != nil:
		return common.WarningEmoji + i.v.ResourceID
	case i.v.ImportError != nil:
		return common.ErrorEmoji + i.v.ResourceID
	case i.v.Imported:
		return common.OKEmoji + i.v.ResourceID
	default:
		if i.v.IsRecommended {
			return common.BulbEmoji + i.v.ResourceID
		}
		return i.v.ResourceID
	}
}

func (i Item) Description() string {
	if i.textinput.Focused() {
		return i.textinput.View()
	}
	if i.v.Skip() {
		return "(Skip)"
	}
	return i.textinput.Value()
}

func (i Item) FilterValue() string {
	if i.v.ValidateError == nil && i.v.ImportError == nil && !i.v.Imported && !i.v.IsRecommended {
		return i.v.ResourceID
	}
	return " " + i.v.ResourceID
}
