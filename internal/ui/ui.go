package ui

import (
	"context"
	"fmt"

	"github.com/Azure/aztfexport/internal/config"
	"github.com/Azure/aztfexport/internal/log"
	internalmeta "github.com/Azure/aztfexport/internal/meta"
	"github.com/Azure/aztfexport/pkg/meta"

	"github.com/Azure/aztfexport/internal/ui/aztfexportclient"
	"github.com/Azure/aztfexport/internal/ui/common"
	"github.com/mitchellh/go-wordwrap"

	"github.com/muesli/reflow/indent"

	"github.com/Azure/aztfexport/internal/ui/importlist"
	"github.com/Azure/aztfexport/internal/ui/progress"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

const indentLevel = 2

func NewProgram(ctx context.Context, cfg config.InteractiveModeConfig) (*tea.Program, error) {
	m, err := newModel(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return tea.NewProgram(m, tea.WithAltScreen()), nil
}

type status int

const (
	statusInit status = iota
	statusListingResource
	statusBuildingImportList
	statusImporting
	statusImportErrorMsg
	statusGeneratingCfg
	statusCleaningUpWorkspaceCfg
	statusPushState
	statusExportResourceMapping
	statusExportSkippedResources
	statusSummary
	statusQuitting
	statusError
)

func (s status) String() string {
	return [...]string{
		"initializing",
		"listing Azure resources",
		"building import list",
		"importing",
		"import error message",
		"generating Terraform configuration",
		"cleaning up output directory",
		"pushing state",
		"exporting resource mapping file",
		"exporting skipped resources file",
		"summary",
		"quitting",
		"error",
	}[s]
}

type model struct {
	ctx         context.Context
	meta        meta.Meta
	parallelism int

	status status
	err    error

	// winsize is used to keep track of current windows size, it is used to set the size for other models that are initialized in status (e.g. the importlist).
	winsize tea.WindowSizeMsg

	spinner        spinner.Model
	importlist     importlist.Model
	progress       progress.Model
	importerrormsg aztfexportclient.ShowImportErrorMsg
}

func newModel(ctx context.Context, cfg config.InteractiveModeConfig) (*model, error) {
	s := spinner.NewModel()
	s.Spinner = common.Spinner

	var c meta.Meta = internalmeta.NewGroupMetaDummy(cfg.ResourceGroupName, cfg.ProviderName)
	if !cfg.MockMeta {
		var err error
		c, err = meta.NewMeta(cfg.Config)
		if err != nil {
			return nil, err
		}
	}

	m := &model{
		ctx:         ctx,
		meta:        c,
		parallelism: cfg.Parallelism,
		status:      statusInit,
		spinner:     s,
	}

	return m, nil
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		aztfexportclient.NewClient(m.meta),
		spinner.Tick,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(spinner.TickMsg); !ok {
		m.meta.Logger().Log(context.Background(), log.LevelTrace, "UI update", "status", m.status, "msg", fmt.Sprintf("%#v", msg))
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.winsize = msg
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.status = statusQuitting
			return m, aztfexportclient.Quit(m.ctx, m.meta)
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case aztfexportclient.NewClientMsg:
		m.meta = msg
		m.status = statusInit
		return m, aztfexportclient.Init(m.ctx, m.meta)
	case aztfexportclient.InitProviderDoneMsg:
		m.status = statusListingResource
		return m, aztfexportclient.ListResource(m.ctx, m.meta)
	case aztfexportclient.ListResourceDoneMsg:
		m.status = statusBuildingImportList
		m.importlist = importlist.NewModel(m.ctx, m.meta, msg.List, 0)
		// Trigger a windows resize cmd to resize the importlist model.
		// Though we can pass the winsize as input variable during model initialization.
		// But this way we only need to maintain the resizing logic at one place (which takes consideration of the title height).
		cmd := func() tea.Msg { return m.winsize }
		return m, cmd
	case aztfexportclient.ShowImportErrorMsg:
		m.status = statusImportErrorMsg
		m.importerrormsg = msg
		return m, nil
	case aztfexportclient.StartImportMsg:
		m.status = statusImporting
		m.progress = progress.NewModel(m.ctx, m.meta, m.parallelism, msg.List)
		return m, tea.Batch(
			m.progress.Init(),
			// Resize the progress bar
			func() tea.Msg { return m.winsize },
		)
	case aztfexportclient.ImportDoneMsg:
		for idx, item := range msg.List {
			if item.ImportError != nil {
				m.status = statusBuildingImportList
				m.importlist = importlist.NewModel(m.ctx, m.meta, msg.List, idx)
				cmd := func() tea.Msg { return m.winsize }
				return m, cmd
			}
		}
		m.status = statusPushState
		return m, aztfexportclient.PushState(m.ctx, m.meta, msg.List)
	case aztfexportclient.PushStateDoneMsg:
		m.status = statusExportResourceMapping
		return m, aztfexportclient.ExportResourceMapping(m.ctx, m.meta, msg.List)
	case aztfexportclient.ExportResourceMappingDoneMsg:
		m.status = statusExportSkippedResources
		return m, aztfexportclient.ExportSkippedResources(m.ctx, m.meta, msg.List)
	case aztfexportclient.ExportSkippedResourcesDoneMsg:
		m.status = statusGeneratingCfg
		return m, aztfexportclient.GenerateCfg(m.ctx, m.meta, msg.List)
	case aztfexportclient.GenerateCfgDoneMsg:
		m.status = statusCleaningUpWorkspaceCfg
		return m, aztfexportclient.CleanUpWorkspace(m.ctx, m.meta)
	case aztfexportclient.WorkspaceCleanupDoneMsg:
		m.status = statusSummary
		return m, nil
	case aztfexportclient.QuitMsg:
		return m, tea.Quit
	case aztfexportclient.CleanTFStateMsg:
		m.meta.CleanTFState(m.ctx, msg.Addr)
		return m, nil
	case aztfexportclient.ErrMsg:
		m.status = statusError
		m.err = msg
		return m, nil
	}

	return updateChildren(msg, m)
}

func updateChildren(msg tea.Msg, m model) (model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.status {
	case statusBuildingImportList:
		m.importlist, cmd = m.importlist.Update(msg)
		return m, cmd
	case statusImportErrorMsg:
		if _, ok := msg.(tea.KeyMsg); ok {
			m.status = statusBuildingImportList
			m.importlist = importlist.NewModel(m.ctx, m.meta, m.importerrormsg.List, m.importerrormsg.Index)
			cmd = func() tea.Msg { return m.winsize }
			return m, cmd
		}
	case statusImporting:
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd
	case statusSummary:
		switch msg.(type) {
		case tea.KeyMsg:
			m.status = statusQuitting
			return m, aztfexportclient.Quit(m.ctx, m.meta)
		}
	}
	return m, nil
}

func (m model) View() string {
	s := m.logoView()

	switch m.status {
	case statusInit:
		s += m.spinner.View() + " Initializing..."
	case statusListingResource:
		s += m.spinner.View() + " Listing Azure Resources..."
	case statusBuildingImportList:
		s += m.importlist.View()
	case statusImportErrorMsg:
		s += importErrorView(m)
	case statusImporting:
		s += m.spinner.View() + m.progress.View()
	case statusPushState:
		s += m.spinner.View() + " Pushing Terraform Status..."
	case statusExportResourceMapping:
		s += m.spinner.View() + " Exporting Resource Mapping..."
	case statusExportSkippedResources:
		s += m.spinner.View() + " Exporting Skipped Resources..."
	case statusGeneratingCfg:
		s += m.spinner.View() + " Generating Terraform Configurations..."
	case statusCleaningUpWorkspaceCfg:
		s += m.spinner.View() + " Cleaning up the output directory..."
	case statusSummary:
		s += summaryView(m)
	case statusError:
		s += errorView(m)
	}

	return indent.String(s, indentLevel)
}

func (m model) logoView() string {
	return "\n" + common.TitleStyle.Render(" Microsoft Azure Export for Terraform ") + "\n\n"
}

func importErrorView(m model) string {
	// #nosec G115
	return m.importerrormsg.Item.TFResourceId + "\n\n" + common.ErrorMsgStyle.Render(wordwrap.WrapString(m.importerrormsg.Item.ImportError.Error(), uint(m.winsize.Width-indentLevel)))
}

func summaryView(m model) string {
	return fmt.Sprintf("Terraform state and the config are generated at: %s\n\n", m.meta.Workspace()) + common.QuitMsgStyle.Render("Press any key to quit\n")
}

func errorView(m model) string {
	// #nosec G115
	return common.ErrorMsgStyle.Render(wordwrap.WrapString(m.err.Error(), uint(m.winsize.Width-indentLevel)))
}
