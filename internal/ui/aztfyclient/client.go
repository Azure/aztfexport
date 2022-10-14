package aztfyclient

import (
	"time"

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

type ImportItemsDoneMsg struct {
	Items []meta.ImportItem
}

type ImportDoneMsg struct {
	List meta.ImportList
}

type ExportResourceMappingDoneMsg struct {
	List meta.ImportList
}

type ExportSkippedResourcesDoneMsg struct {
	List meta.ImportList
}

type GenerateCfgDoneMsg struct{}

type WorkspaceCleanupDoneMsg struct{}

type QuitMsg struct{}

type CleanTFStateMsg struct {
	Addr string
}

func NewClient(meta meta.Meta) tea.Cmd {
	return func() tea.Msg {
		return NewClientMsg(meta)
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

func ListResource(c meta.Meta) tea.Cmd {
	return func() tea.Msg {
		list, err := c.ListResource()
		if err != nil {
			return ErrMsg(err)
		}
		return ListResourceDoneMsg{List: list}
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

func ImportItems(c meta.Meta, items []meta.ImportItem) tea.Cmd {
	return func() tea.Msg {
		var l []*meta.ImportItem
		for i := range items {
			if items[i].Skip() || items[i].Imported {
				continue
			}
			l = append(l, &items[i])
		}
		c.ParallelImport(l)
		return ImportItemsDoneMsg{Items: items}
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

func CleanUpWorkspace(c meta.Meta) tea.Cmd {
	return func() tea.Msg {
		if err := c.CleanUpWorkspace(); err != nil {
			return ErrMsg(err)
		}
		return WorkspaceCleanupDoneMsg{}
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

func ExportSkippedResources(c meta.Meta, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		if err := c.ExportSkippedResources(l); err != nil {
			return ErrMsg(err)
		}
		return ExportSkippedResourcesDoneMsg{List: l}
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
