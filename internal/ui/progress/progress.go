package progress

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/magodo/aztfy/internal/meta"
	"github.com/magodo/aztfy/internal/ui/aztfyclient"
	common2 "github.com/magodo/aztfy/internal/ui/common"
)

type result struct {
	item  meta.ImportItem
	emoji string
}

type Model struct {
	c       meta.Meta
	l       meta.ImportList
	idx     int
	results []result
}

func NewModel(c meta.Meta, l meta.ImportList) Model {
	return Model{
		c:       c,
		l:       l,
		idx:     0,
		results: make([]result, common2.ProgressShowLastResults),
	}
}

func (m Model) Init() tea.Cmd {
	if m.iterationDone() {
		return aztfyclient.FinishImport(m.l)
	}

	return tea.Batch(
		aztfyclient.ImportOneItem(m.c, m.l[m.idx]),
	)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case aztfyclient.ImportOneItemDoneMsg:
		item := msg.Item
		m.l[m.idx] = item
		res := result{
			item: msg.Item,
		}
		if item.ImportError != nil {
			res.emoji = common2.WarningEmoji
		} else {
			res.emoji = common2.RandomHappyEmoji()
		}
		m.results = append(m.results[1:], res)
		m.idx++
		if m.iterationDone() {
			return m, aztfyclient.FinishImport(m.l)
		}
		return m, aztfyclient.ImportOneItem(m.c, m.l[m.idx])
	default:
		return m, nil
	}
}

func (m Model) View() string {
	msg := ""
	if len(m.l) > m.idx {
		item := m.l[m.idx]
		if item.Skip() {
			msg = fmt.Sprintf(" Skipping %s...", item.ResourceID)
		} else {
			msg = fmt.Sprintf(" Importing %s...", item.ResourceID)
		}
	}

	s := fmt.Sprintf(" %s\n\n", msg)

	for _, res := range m.results {
		emptyItem := meta.ImportItem{}
		if res.item == emptyItem {
			s += "........................\n"
		} else {
			switch {
			case res.item.Skip():
				s += fmt.Sprintf("%s %s skipped\n", res.emoji, res.item.ResourceID)
			case res.item.ImportError == nil:
				s += fmt.Sprintf("%s %s import successfully\n", res.emoji, res.item.ResourceID)
			case res.item.ImportError != nil:
				s += fmt.Sprintf("%s %s import failed\n", res.emoji, res.item.ResourceID)
			}
		}
	}

	return s
}

func (m Model) iterationDone() bool {
	return len(m.l) == m.idx
}
