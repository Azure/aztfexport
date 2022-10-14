package progress

import (
	"fmt"

	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/internal/ui/aztfyclient"
	"github.com/Azure/aztfy/internal/ui/common"
	prog "github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
)

type result struct {
	item  meta.ImportItem
	emoji string
}

type Model struct {
	c meta.Meta
	l meta.ImportList

	idx            int
	parallelImport bool
	parallelism    int

	results  []result
	progress prog.Model
}

func NewModel(c meta.Meta, parallelImport bool, parallelism int, l meta.ImportList) Model {
	return Model{
		c:              c,
		l:              l,
		idx:            0,
		parallelImport: parallelImport,
		parallelism:    parallelism,
		results:        make([]result, common.ProgressShowLastResults),
		progress:       prog.NewModel(prog.WithDefaultGradient()),
	}
}

func (m Model) Init() tea.Cmd {
	if m.iterationDone() {
		return aztfyclient.FinishImport(m.l)
	}

	if m.parallelImport {
		n := m.parallelism
		if m.idx+m.parallelism > len(m.l) {
			n = len(m.l) - m.idx
		}
		return tea.Batch(
			aztfyclient.ImportItems(m.c, m.l[m.idx:m.idx+n]),
		)
	}

	return tea.Batch(
		aztfyclient.ImportOneItem(m.c, m.l[m.idx]),
	)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.progress.Width = msg.Width - 4
		return m, nil
	// FrameMsg is sent when the progress bar wants to animate itself
	case prog.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(prog.Model)
		return m, cmd
	case aztfyclient.ImportItemsDoneMsg:
		var cmds []tea.Cmd

		// Update results
		items := msg.Items
		for i := range items {
			m.l[m.idx+i] = items[i]

			emoji := common.RandomHappyEmoji()
			if items[i].ImportError != nil {
				emoji = common.WarningEmoji
			}
			res := result{
				item:  items[i],
				emoji: emoji,
			}
			m.results = append(m.results[1:], res)
		}

		m.idx += m.parallelism

		if m.iterationDone() {
			cmds = append(cmds, m.progress.SetPercent(1))
			cmds = append(cmds, aztfyclient.FinishImport(m.l))
			return m, tea.Batch(cmds...)
		}

		cmds = append(cmds, m.progress.SetPercent(float64(m.idx)/float64(len(m.l))))

		n := m.parallelism
		if m.idx+m.parallelism > len(m.l) {
			n = len(m.l) - m.idx
		}
		cmds = append(cmds, aztfyclient.ImportItems(m.c, m.l[m.idx:m.idx+n]))
		return m, tea.Batch(cmds...)

	case aztfyclient.ImportOneItemDoneMsg:
		var cmds []tea.Cmd

		// Update results
		item := msg.Item
		m.l[m.idx] = item
		res := result{
			item: msg.Item,
		}
		if item.ImportError != nil {
			res.emoji = common.WarningEmoji
		} else {
			res.emoji = common.RandomHappyEmoji()
		}
		m.results = append(m.results[1:], res)

		m.idx++

		cmds = append(cmds, m.progress.SetPercent(float64(m.idx)/float64(len(m.l))))

		// Import the next
		if m.iterationDone() {
			cmds = append(cmds, aztfyclient.FinishImport(m.l))
			return m, tea.Batch(cmds...)
		}
		cmds = append(cmds, aztfyclient.ImportOneItem(m.c, m.l[m.idx]))
		return m, tea.Batch(cmds...)
	default:
		return m, nil
	}
}

func (m Model) View() string {
	msg := ""
	if len(m.l) > m.idx {
		item := m.l[m.idx]
		if item.Skip() {
			msg = fmt.Sprintf(" Skipping %s...", item.TFResourceId)
		} else {
			msg = fmt.Sprintf(" Importing %s...", item.TFResourceId)
		}
	}

	s := fmt.Sprintf(" %s\n\n", msg)
	for _, res := range m.results {
		// This indicates the state before the item is inserted as the to results.
		if res.item.TFResourceId == "" {
			s += "...\n"
		} else {
			switch {
			case res.item.Skip():
				s += fmt.Sprintf("%s %s skipped\n", res.emoji, res.item.TFResourceId)
			default:
				if res.item.ImportError == nil {
					s += fmt.Sprintf("%s %s import successfully\n", res.emoji, res.item.TFResourceId)
				} else {
					s += fmt.Sprintf("%s %s import failed\n", res.emoji, res.item.TFResourceId)
				}
			}
		}
	}

	s += "\n\n" + m.progress.View()

	return s
}

func (m Model) iterationDone() bool {
	return m.idx >= len(m.l)
}
