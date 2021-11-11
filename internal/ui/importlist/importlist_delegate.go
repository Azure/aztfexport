package importlist

import (
	"errors"
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
				rt, rn, err := parseInput(selItem.textinput.Value())
				if err != nil {
					cmd := m.NewStatusMessage(common.ErrorMsgStyle.Render(err.Error()))
					cmds = append(cmds, cmd)
					selItem.v.ValidateError = err
					return
				}

				// Validate there is no resource type has duplicate resource names

				// NOTE: we should cache this map somewhere and update it during user's interactions.
				// Whilst, there is no means for a list delegation to have such cache capability. Therefore, we are building a refresh map here.
				resourceMap := map[string]map[string]bool{}
				for _, item := range m.Items() {
					item := item.(Item)
					v := item.v
					if v.Skip() {
						continue
					}
					if item.idx == selItem.idx {
						continue
					}
					namesMap, ok := resourceMap[v.TFResourceType]
					if !ok {
						namesMap = map[string]bool{}
						resourceMap[v.TFResourceType] = namesMap
					}
					namesMap[v.TFResourceName] = true
				}
				if namesMap, ok := resourceMap[rt]; ok {
					if _, ok := namesMap[rn]; ok {
						err := fmt.Errorf("%q has duplcate name as %q", rt, rn)
						cmd := m.NewStatusMessage(common.ErrorMsgStyle.Render(err.Error()))
						cmds = append(cmds, cmd)
						selItem.v.ValidateError = err
						return
					}
				}
				selItem.v.TFResourceType, selItem.v.TFResourceName = rt, rn

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

func parseInput(input string) (restype, resname string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", nil
	}

	segs := strings.Split(input, ".")
	if len(segs) != 2 {
		return "", "", errors.New(`Invalid input format, expect "<resource type>.<resource name>"`)
	}

	rt, rn := segs[0], segs[1]
	if rt == "" || rn == "" {
		return "", "", errors.New(`Invalid input format, expect "<resource type>.<resource name>"`)
	}

	if _, ok := schema.ProviderSchemaInfo.ResourceSchemas[rt]; !ok {
		return "", "", fmt.Errorf("Invalid resource type %q", rt)
	}

	return rt, rn, nil
}
