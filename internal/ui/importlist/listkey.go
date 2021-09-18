package importlist

import "github.com/charmbracelet/bubbles/key"

type listKeyMap struct {
	apply key.Binding
	error key.Binding
}

func newListKeyMap() listKeyMap {
	return listKeyMap{
		apply: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "apply"),
		),
		error: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "show error"),
		),
	}
}

func (m listKeyMap) ToBindings() []key.Binding {
	return []key.Binding{
		m.apply,
		m.error,
	}
}
