package aztfexportclient

import (
	"context"

	"github.com/Azure/aztfexport/pkg/meta"

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

type ImportItemsDoneMsg struct {
	Items []meta.ImportItem
}

type ImportDoneMsg struct {
	List meta.ImportList
}

type PushStateDoneMsg struct {
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

func Init(ctx context.Context, c meta.Meta) tea.Cmd {
	return func() tea.Msg {
		err := c.Init(ctx)
		if err != nil {
			return ErrMsg(err)
		}
		return InitProviderDoneMsg{}
	}
}

func ListResource(ctx context.Context, c meta.Meta) tea.Cmd {
	return func() tea.Msg {
		list, err := c.ListResource(ctx)
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

func StartImport(l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		return StartImportMsg{List: l}
	}
}

func ImportItems(ctx context.Context, c meta.Meta, items []meta.ImportItem) tea.Cmd {
	return func() tea.Msg {
		var l []*meta.ImportItem
		for i := range items {
			if items[i].Skip() || items[i].Imported {
				continue
			}
			l = append(l, &items[i])
		}
		if err := c.ParallelImport(ctx, l); err != nil {
			return ErrMsg(err)
		}
		return ImportItemsDoneMsg{Items: items}
	}
}

func FinishImport(l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		return ImportDoneMsg{List: l}
	}
}

func GenerateCfg(ctx context.Context, c meta.Meta, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		if err := c.GenerateCfg(ctx, l); err != nil {
			return ErrMsg(err)
		}
		return GenerateCfgDoneMsg{}
	}
}

func CleanUpWorkspace(ctx context.Context, c meta.Meta) tea.Cmd {
	return func() tea.Msg {
		if err := c.CleanUpWorkspace(ctx); err != nil {
			return ErrMsg(err)
		}
		return WorkspaceCleanupDoneMsg{}
	}
}

func PushState(ctx context.Context, c meta.Meta, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		if err := c.PushState(ctx); err != nil {
			return ErrMsg(err)
		}
		return PushStateDoneMsg{List: l}
	}
}

func ExportResourceMapping(ctx context.Context, c meta.Meta, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		if err := c.WriteResourceMapping(ctx, l); err != nil {
			return ErrMsg(err)
		}
		return ExportResourceMappingDoneMsg{List: l}
	}
}

func ExportSkippedResources(ctx context.Context, c meta.Meta, l meta.ImportList) tea.Cmd {
	return func() tea.Msg {
		if err := c.WriteSkippedResources(ctx, l); err != nil {
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

func Quit(ctx context.Context, c meta.Meta) tea.Cmd {
	return func() tea.Msg {
		if err := c.DeInit(ctx); err != nil {
			return ErrMsg(err)
		}
		return QuitMsg{}
	}
}
