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
	c        meta.RgMeta
	l        meta.ImportList
	idx      int
	results  []result
	progress prog.Model
}

func NewModel(c meta.RgMeta, l meta.ImportList) Model {
	return Model{
		c:        c,
		l:        l,
		idx:      0,
		results:  make([]result, common.ProgressShowLastResults),
		progress: prog.NewModel(prog.WithDefaultGradient()),
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
	case tea.WindowSizeMsg:
		m.progress.Width = msg.Width - 4
		return m, nil
	// FrameMsg is sent when the progress bar wants to animate itself
	case prog.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(prog.Model)
		return m, cmd
	case aztfyclient.ImportOneItemDoneMsg:
		var cmds []tea.Cmd
		var cmd tea.Cmd

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

		// Update progress bar
		cmd = m.progress.SetPercent(float64(m.idx+1) / float64(len(m.l)))
		cmds = append(cmds, cmd)

		m.idx++
		if m.iterationDone() {
			cmd = aztfyclient.FinishImport(m.l)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}
		cmd = aztfyclient.ImportOneItem(m.c, m.l[m.idx])
		cmds = append(cmds, cmd)
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
			msg = fmt.Sprintf(" Skipping %s...", item.ResourceID)
		} else {
			msg = fmt.Sprintf(" Importing %s...", item.ResourceID)
		}
	}

	s := fmt.Sprintf(" %s\n\n", msg)
	for _, res := range m.results {
		// This indicates the state before the item is inserted as the to results.
		if res.item.ResourceID == "" {
			s += "...\n"
		} else {
			switch {
			case res.item.Skip():
				s += fmt.Sprintf("%s %s skipped\n", res.emoji, res.item.ResourceID)
			default:
				if res.item.ImportError == nil {
					s += fmt.Sprintf("%s %s import successfully\n", res.emoji, res.item.ResourceID)
				} else {
					s += fmt.Sprintf("%s %s import failed\n", res.emoji, res.item.ResourceID)
				}
			}
		}
	}

	s += "\n\n" + m.progress.View()

	return s
}

func (m Model) iterationDone() bool {
	return len(m.l) == m.idx
}
