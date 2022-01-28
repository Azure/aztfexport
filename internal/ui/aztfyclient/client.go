package aztfyclient

import (
	"time"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/meta"

	tea "github.com/charmbracelet/bubbletea"
)

type NewClientMsg meta.Meta

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

type ExportResourceMappingDoneMsg struct {
	List meta.ImportList
}

type GenerateCfgDoneMsg struct{}

type QuitMsg struct{}

type CleanTFStateMsg struct {
	Addr string
}

func NewClient(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		c, err := meta.NewMeta(cfg)
		if err != nil {
			return ErrMsg(err)
		}
		return NewClientMsg(c)
	}
}

func Init(c meta.Meta) tea.Cmd {
	return func() tea.Msg {
		err := c.Init()
		if err != nil {
			return ErrMsg(err)
		}
		return InitProviderDoneMsg{}
	}
}

func ListResource(c meta.Meta, resourceMapping map[string]string) tea.Cmd {
	return func() tea.Msg {
		return ListResourceDoneMsg{List: c.ListResource()}
	}
}

func ShowImportError(item meta.ImportItem, idx int, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		return ShowImportErrorMsg{Item: item, Index: idx, List: l}
	}
}

func StartImport(c meta.Meta, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		return StartImportMsg{List: l}
	}
}

func ImportOneItem(c meta.Meta, item meta.ImportItem) tea.Cmd {
	return func() tea.Msg {
		if !item.Skip() && !item.Imported {
			c.Import(&item)
		} else {
			// This explicit minor delay is for the sake of a visual effect of the progress bar.
			time.Sleep(100 * time.Millisecond)
		}
		return ImportOneItemDoneMsg{Item: item}
	}
}

func FinishImport(l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		return ImportDoneMsg{List: l}
	}
}

func GenerateCfg(c meta.Meta, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		if err := c.GenerateCfg(l); err != nil {
			return ErrMsg(err)
		}
		return GenerateCfgDoneMsg{}
	}
}

func ExportResourceMapping(c meta.Meta, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		if err := c.ExportResourceMapping(l); err != nil {
			return ErrMsg(err)
		}
		return ExportResourceMappingDoneMsg{List: l}
	}
}

func CleanTFState(addr string) tea.Cmd {
	return func() tea.Msg {
		return CleanTFStateMsg{addr}
	}
}

func Quit() tea.Cmd {
	return func() tea.Msg {
		return QuitMsg{}
	}
}
