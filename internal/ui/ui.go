package ui

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/internal/ui/aztfyclient"
	"github.com/Azure/aztfy/internal/ui/common"
	"github.com/mitchellh/go-wordwrap"

	"github.com/muesli/reflow/indent"

	"github.com/Azure/aztfy/internal/ui/importlist"
	"github.com/Azure/aztfy/internal/ui/progress"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

const indentLevel = 2

func NewProgram(cfg config.Config) (*tea.Program, error) {
	// Discard logs from hashicorp/azure-go-helper
	log.SetOutput(io.Discard)

	// Define another dedicated logger for the ui
	logger := log.New(os.Stderr, "", log.LstdFlags)
	if cfg.Logfile != "" {
		f, err := os.OpenFile(cfg.Logfile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		logger = log.New(f, "aztfy", log.LstdFlags)
	}
	m, err := newModel(cfg, logger)
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
	statusExportResourceMapping
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
		"summary",
		"quitting",
		"error",
	}[s]
}

type model struct {
	meta   meta.Meta
	debug  bool
	status status
	err    error
	logger *log.Logger

	// winsize is used to keep track of current windows size, it is used to set the size for other models that are initialized in status (e.g. the importlist).
	winsize tea.WindowSizeMsg

	spinner        spinner.Model
	importlist     importlist.Model
	progress       progress.Model
	importerrormsg aztfyclient.ShowImportErrorMsg
}

func newModel(cfg config.Config, logger *log.Logger) (*model, error) {
	s := spinner.NewModel()
	s.Spinner = common.Spinner

	m := &model{
		debug:   cfg.Debug,
		status:  statusInit,
		logger:  logger,
		spinner: s,
	}
	meta, err := meta.NewMeta(cfg)
	if err != nil {
		return nil, err
	}
	m.meta = meta

	return m, nil
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		aztfyclient.NewClient(m.meta),
		spinner.Tick,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.debug {
		if _, ok := msg.(spinner.TickMsg); !ok {
			m.logger.Printf("STATUS: %s | MSG: %#v\n", m.status, msg)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.winsize = msg
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.status = statusQuitting
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case aztfyclient.NewClientMsg:
		m.meta = msg
		m.status = statusInit
		return m, aztfyclient.Init(m.meta)
	case aztfyclient.InitProviderDoneMsg:
		m.status = statusListingResource
		return m, aztfyclient.ListResource(m.meta)
	case aztfyclient.ListResourceDoneMsg:
		m.status = statusBuildingImportList
		m.importlist = importlist.NewModel(m.meta, msg.List, 0)
		// Trigger a windows resize cmd to resize the importlist model.
		// Though we can pass the winsize as input variable during model initialization.
		// But this way we only need to maintain the resizing logic at one place (which takes consideration of the title height).
		cmd := func() tea.Msg { return m.winsize }
		return m, cmd
	case aztfyclient.ShowImportErrorMsg:
		m.status = statusImportErrorMsg
		m.importerrormsg = msg
		return m, nil
	case aztfyclient.StartImportMsg:
		m.status = statusImporting
		m.progress = progress.NewModel(m.meta, msg.List)
		return m, tea.Batch(
			m.progress.Init(),
			// Resize the progress bar
			func() tea.Msg { return m.winsize },
		)
	case aztfyclient.ImportDoneMsg:
		for idx, item := range msg.List {
			if item.ImportError != nil {
				m.status = statusBuildingImportList
				m.importlist = importlist.NewModel(m.meta, msg.List, idx)
				cmd := func() tea.Msg { return m.winsize }
				return m, cmd
			}
		}
		m.status = statusExportResourceMapping
		return m, aztfyclient.ExportResourceMapping(m.meta, msg.List)
	case aztfyclient.ExportResourceMappingDoneMsg:
		m.status = statusGeneratingCfg
		return m, aztfyclient.GenerateCfg(m.meta, msg.List)
	case aztfyclient.GenerateCfgDoneMsg:
		m.status = statusSummary
		return m, nil
	case aztfyclient.QuitMsg:
		return m, tea.Quit
	case aztfyclient.CleanTFStateMsg:
		m.meta.CleanTFState(msg.Addr)
		return m, nil
	case aztfyclient.ErrMsg:
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
			m.importlist = importlist.NewModel(m.meta, m.importerrormsg.List, m.importerrormsg.Index)
			cmd = func() tea.Msg { return m.winsize }
			return m, cmd
		}
	case statusImporting:
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd
	case statusSummary:
		switch msg.(type) {
		case tea.KeyMsg:
			return m, aztfyclient.Quit()
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
		s += m.spinner.View() + " Listing Azure Resources reside in " + `"` + m.meta.ResourceGroupName() + `"...`
	case statusBuildingImportList:
		s += m.importlist.View()
	case statusImportErrorMsg:
		s += importErrorView(m)
	case statusImporting:
		s += m.spinner.View() + m.progress.View()
	case statusGeneratingCfg:
		s += m.spinner.View() + " Generating Terraform Configurations..."
	case statusExportResourceMapping:
		s += m.spinner.View() + " Exporting Resource Mapping..."
	case statusSummary:
		s += summaryView(m)
	case statusError:
		s += errorView(m)
	}

	return indent.String(s, indentLevel)
}

func (m model) logoView() string {
	return "\n" + common.TitleStyle.Render(" Azure Terrafy ") + "\n\n"
}

func importErrorView(m model) string {
	return m.importerrormsg.Item.ResourceID + "\n\n" + common.ErrorMsgStyle.Render(wordwrap.WrapString(m.importerrormsg.Item.ImportError.Error(), uint(m.winsize.Width-indentLevel)))
}

func summaryView(m model) string {
	return fmt.Sprintf("Terraform state and the config are generated at: %s\n\n", m.meta.Workspace()) + common.QuitMsgStyle.Render("Press any key to quit\n")
}

func errorView(m model) string {
	return common.ErrorMsgStyle.Render(wordwrap.WrapString(m.err.Error(), uint(m.winsize.Width-indentLevel)))
}
