package importlist

import (
	"fmt"
	"strings"

	"github.com/Azure/aztfy/internal/ui/common"
	"github.com/Azure/aztfy/schema"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func NewImportItemDelegate() list.ItemDelegate {
	d := list.NewDefaultDelegate()
	d.UpdateFunc = func(msg tea.Msg, m *list.Model) (ret tea.Cmd) {
		sel := m.SelectedItem()
		if sel == nil {
			return nil
		}
		selItem := sel.(Item)
		idx := selItem.idx

		var cmds []tea.Cmd
		defer func() {
			cmd := m.SetItem(idx, selItem)
			cmds = append(cmds, cmd)
			ret = tea.Batch(cmds...)
		}()

		// For the item that is not focused (i.e. the textinput is not focused)
		if !selItem.textinput.Focused() {
			switch msg := msg.(type) {
			case tea.KeyMsg:
				switch {
				case msg.Type == tea.KeyEnter:
					// Clear the validation error that were set.
					selItem.v.ValidateError = nil

					// "Enter" focus current selected item
					setListKeyMapEnabled(m, false)
					cmd := selItem.textinput.Focus()
					cmds = append(cmds, cmd)
					return
				}
			}
			return
		}

		// The item is focused.
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter,
				tea.KeyEsc:
				// Enter and ESC un-focus the textinput
				setListKeyMapEnabled(m, true)
				selItem.textinput.Blur()

				// Validate the input and update the selItem.v
				rt, err := parseInput(selItem.textinput.Value())
				if err != nil {
					cmd := m.NewStatusMessage(common.ErrorMsgStyle.Render(err.Error()))
					cmds = append(cmds, cmd)
					selItem.v.ValidateError = err
					return
				}
				selItem.v.TFResourceType = rt
				return
			}
		}

		var cmd tea.Cmd
		selItem.textinput, cmd = selItem.textinput.Update(msg)
		cmds = append(cmds, cmd)
		return
	}
	return d
}

func setListKeyMapEnabled(m *list.Model, enabled bool) {
	m.KeyMap.CursorUp.SetEnabled(enabled)
	m.KeyMap.CursorDown.SetEnabled(enabled)
	m.KeyMap.NextPage.SetEnabled(enabled)
	m.KeyMap.PrevPage.SetEnabled(enabled)
	m.KeyMap.GoToStart.SetEnabled(enabled)
	m.KeyMap.GoToEnd.SetEnabled(enabled)

	m.KeyMap.Filter.SetEnabled(enabled)
	if enabled {
		m.KeyMap.ClearFilter.SetEnabled(m.FilterState() == list.FilterApplied)
	} else {
		m.KeyMap.ClearFilter.SetEnabled(enabled)
	}

	// m.KeyMap.CancelWhileFiltering.SetEnabled(enabled)
	// m.KeyMap.AcceptWhileFiltering.SetEnabled(enabled)

	m.KeyMap.ShowFullHelp.SetEnabled(enabled)
	m.KeyMap.CloseFullHelp.SetEnabled(enabled)

	m.KeyMap.Quit.SetEnabled(enabled)

	if enabled {
		bindKeyHelps(m, newListKeyMap().ToBindings())
	} else {
		bindKeyHelps(m, nil)
	}
}

func parseInput(input string) (restype string, err error) {
	rt := strings.TrimSpace(input)
	if rt == "" {
		return "", nil
	}

	if _, ok := schema.ProviderSchemaInfo.ResourceSchemas[rt]; !ok {
		return "", fmt.Errorf("Invalid resource type %q", rt)
	}

	return rt, nil
}
