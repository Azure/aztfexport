package importlist

import (
	"github.com/Azure/aztfexport/internal/ui/common"
	"github.com/Azure/aztfexport/pkg/meta"
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
		return common.WarningEmoji + i.v.TFResourceId
	case i.v.ImportError != nil:
		return common.ErrorEmoji + i.v.TFResourceId
	case i.v.Imported:
		return common.OKEmoji + i.v.TFResourceId
	default:
		if i.v.IsRecommended {
			return common.BulbEmoji + i.v.TFResourceId
		}
		return i.v.TFResourceId
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
		return i.v.TFResourceId
	}
	return " " + i.v.TFResourceId
}
