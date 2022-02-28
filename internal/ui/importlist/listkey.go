package importlist

import "github.com/charmbracelet/bubbles/key"

type listKeyMap struct {
	apply          key.Binding
	error          key.Binding
	recommendation key.Binding
	save           key.Binding
}

func newListKeyMap() listKeyMap {
	return listKeyMap{
		apply: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "import"),
		),
		error: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "show error"),
		),
		recommendation: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "show recommendation"),
		),
		save: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "save"),
		),
	}
}

func (m listKeyMap) ToBindings() []key.Binding {
	return []key.Binding{
		m.apply,
		m.error,
		m.recommendation,
		m.save,
	}
}
