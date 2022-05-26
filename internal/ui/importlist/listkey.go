package importlist

import "github.com/charmbracelet/bubbles/key"

type listKeyMap struct {
	skip           key.Binding
	error          key.Binding
	recommendation key.Binding
	apply          key.Binding
	save           key.Binding
}

func newListKeyMap() listKeyMap {
	return listKeyMap{
		skip: key.NewBinding(
			key.WithKeys("delete"),
			key.WithHelp("delete", "skip"),
		),
		error: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "show error"),
		),
		recommendation: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "show recommendation"),
		),
		apply: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "import"),
		),
		save: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "save"),
		),
	}
}

func (m listKeyMap) ToBindings() []key.Binding {
	return []key.Binding{
		m.skip,
		m.error,
		m.recommendation,
		m.apply,
		m.save,
	}
}
