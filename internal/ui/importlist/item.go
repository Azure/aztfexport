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
	default:
		return i.v.ResourceID
	}
}

func (i Item) Description() string {
	if i.textinput.Focused() {
		return i.textinput.View()
	}
	v := i.textinput.Value()
	if v == "" {
		return "(Skip)"
	}
	return v
}

func (i Item) FilterValue() string { return i.v.ResourceID }
