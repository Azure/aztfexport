package aztfyclient

import (
	"context"
	"github.com/magodo/aztfy/internal/meta"

	tea "github.com/charmbracelet/bubbletea"
)

type NewClientMsg *meta.Meta

type ErrMsg error

type InitProviderDoneMsg struct{}

type ListResourceDoneMsg struct {
	List meta.ImportList
}

type ShowImportErrorMsg struct {
	Item  meta.ImportItem
	Index int
	List  meta.ImportList
}

type StartImportMsg struct {
	List meta.ImportList
}

type ImportOneItemDoneMsg struct {
	Item meta.ImportItem
}

type ImportDoneMsg struct {
	List meta.ImportList
}

type GenerateCfgDoneMsg struct{}

type QuitMsg struct{}

func NewClient(rg string) tea.Cmd {
	return func() tea.Msg {
		c, err := meta.NewMeta(context.TODO(), rg)
		if err != nil {
			return ErrMsg(err)
		}
		return NewClientMsg(c)
	}
}

func Init(c *meta.Meta) tea.Cmd {
	return func() tea.Msg {
		err := c.Init(context.TODO())
		if err != nil {
			return ErrMsg(err)
		}
		return InitProviderDoneMsg{}
	}
}

func ListResource(c *meta.Meta) tea.Cmd {
	return func() tea.Msg {
		return ListResourceDoneMsg{List: c.ListResource()}
	}
}

func ShowImportError(item meta.ImportItem, idx int, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		return ShowImportErrorMsg{Item: item, Index: idx, List: l}
	}
}

func StartImport(c *meta.Meta, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		c.CleanTFState()
		return StartImportMsg{List: l}
	}
}

func ImportOneItem(c *meta.Meta, item meta.ImportItem) tea.Cmd {
	return func() tea.Msg {
		if !item.Skip() {
			item.ImportError = c.Import(context.TODO(), item)
		}
		return ImportOneItemDoneMsg{Item: item}
	}
}

func FinishImport(l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		return ImportDoneMsg{List: l}
	}
}

func GenerateCfg(c *meta.Meta, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		if err := c.GenerateCfg(context.TODO(), l); err != nil {
			return ErrMsg(err)
		}
		return GenerateCfgDoneMsg{}
	}
}

func Quit() tea.Cmd {
	return func() tea.Msg {
		return QuitMsg{}
	}
}
