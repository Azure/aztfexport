package importlist

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Azure/aztfy/internal/tfaddr"

	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/internal/ui/aztfyclient"
	"github.com/Azure/aztfy/internal/ui/common"
	"github.com/Azure/aztfy/mapping"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/magodo/textinput"
	"github.com/magodo/tfadd/providers/azurerm"
)

type Model struct {
	c               meta.Meta
	listkeys        listKeyMap
	recommendations [][]string

	list list.Model
}

func NewModel(c meta.Meta, l meta.ImportList, idx int) Model {
	// Build candidate words for the textinput
	candidates := make([]string, 0, len(azurerm.ProviderSchemaInfo.ResourceSchemas))
	for rt := range azurerm.ProviderSchemaInfo.ResourceSchemas {
		candidates = append(candidates, rt)
	}
	sort.Strings(candidates)

	// Build the recommendation for each list item
	recommendations := buildResourceRecommendations(l)

	// Build list items
	var items []list.Item
	for idx, item := range l {
		ti := textinput.NewModel()
		ti.SetCursorMode(textinput.CursorStatic)

		// This only happens on the first time to new the model, where each resource's TFResourceType is empty.
		// Later iterations, this is either a concret resource type or the TFResourceTypeSkip.
		// For this first iteration, we try to give it a recommendation resource type if there is an exact match, otherwise, set it to  TFResourceTypeSkip.
		if item.TFAddr.Type == "" {
			item.TFAddr.Type = tfaddr.TFResourceTypeSkip
			if len(recommendations[idx]) == 1 {
				item.IsRecommended = true
				item.TFAddr.Type = recommendations[idx][0]
			}
		}
		if !item.Skip() {
			ti.SetValue(item.TFAddr.String())
		}
		ti.CandidateWords = candidates
		items = append(items, Item{
			idx:       idx,
			v:         item,
			textinput: ti,
		})
	}

	lst := list.NewModel(items, NewImportItemDelegate(), 0, 0)
	lst.Title = " " + c.ResourceGroupName() + " "
	lst.Styles.Title = common.SubtitleStyle
	lst.StatusMessageLifetime = 3 * time.Second
	lst.Select(idx)
	lst.Filter = func(term string, targets []string) []list.Rank {
		result := []list.Rank{}
		for idx, tgt := range targets {
			if midx := strings.Index(tgt, term); midx != -1 {
				rnk := list.Rank{
					Index: idx,
				}
				for i := 0; i < len(term); i++ {
					rnk.MatchedIndexes = append(rnk.MatchedIndexes, i+midx)
				}
				result = append(result, rnk)
			}
		}
		return result
	}

	bindKeyHelps(&lst, newListKeyMap().ToBindings())

	// Reset the quit to deallocate the "ESC" as a quit key.
	lst.KeyMap.Quit = key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	)

	return Model{
		c:               c,
		listkeys:        newListKeyMap(),
		recommendations: recommendations,

		list: lst,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Don't intercept the keys (e.g. "w") when user is inputting.
		if m.isUserTyping() {
			break
		}

		switch {
		case key.Matches(msg, m.listkeys.apply):
			// Leave filter applied state before applying the import list
			if m.list.FilterState() == list.FilterApplied {
				m.list.ResetFilter()
			}

			// In case all items are marked as skip, show a warning and do nothing.
			if m.isNothingToImport() {
				return m, m.list.NewStatusMessage(common.ErrorMsgStyle.Render("All resources are skipped, nothing to import"))
			}

			// Ensure all items pass validation
			if !m.userInputsAreValid() {
				return m, m.list.NewStatusMessage(common.ErrorMsgStyle.Render("One or more user input is invalid"))
			}

			return m, aztfyclient.StartImport(m.c, m.importList(true))
		case key.Matches(msg, m.listkeys.error):
			sel := m.list.SelectedItem()
			if sel == nil {
				return m, nil
			}
			selItem := sel.(Item)
			if selItem.v.ImportError == nil {
				return m, nil
			}
			return m, aztfyclient.ShowImportError(selItem.v, selItem.idx, m.importList(false))
		case key.Matches(msg, m.listkeys.recommendation):
			sel := m.list.SelectedItem()
			if sel == nil {
				return m, nil
			}
			selItem := sel.(Item)

			recs := m.recommendations[selItem.idx]
			if len(recs) == 0 {
				return m, m.list.NewStatusMessage(common.InfoStyle.Render("No resource type recommendation is avaialble..."))
			}
			return m, m.list.NewStatusMessage(common.InfoStyle.Render(fmt.Sprintf("Possible resource type(s): %s", strings.Join(recs, ","))))
		case key.Matches(msg, m.listkeys.save):
			m.list.NewStatusMessage(common.InfoStyle.Render("Saving the resouce mapping..."))
			err := m.c.ExportResourceMapping(m.importList(false))
			if err == nil {
				m.list.NewStatusMessage(common.InfoStyle.Render(fmt.Sprintf("Resource mapping saved to %s.", meta.ResourceMappingFileName)))
			} else {
				m.list.NewStatusMessage(common.ErrorMsgStyle.Render(err.Error()))
			}
		}
	case tea.WindowSizeMsg:
		// The height here minus the height occupied by the title
		m.list.SetSize(msg.Width, msg.Height-3)
	}
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	return m.list.View()
}

func bindKeyHelps(l *list.Model, bindings []key.Binding) {
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return bindings
	}
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return bindings
	}
}

func (m Model) isUserTyping() bool {
	// In filtering mode
	if m.list.FilterState() == list.Filtering {
		return true
	}

	// Any textinput is in focused mode
	for _, item := range m.list.Items() {
		item := item.(Item)
		if item.textinput.Focused() {
			return true
		}
	}
	return false
}

func (m Model) isNothingToImport() bool {
	for _, item := range m.list.Items() {
		item := item.(Item)
		if !item.v.Skip() {
			return false
		}
	}
	return true
}

func (m Model) userInputsAreValid() bool {
	for _, item := range m.list.Items() {
		item := item.(Item)
		if item.v.ValidateError != nil {
			return false
		}
	}
	return true
}

func (m Model) importList(clearErr bool) meta.ImportList {
	out := make(meta.ImportList, 0, len(m.list.Items()))
	for _, item := range m.list.Items() {
		item := item.(Item)
		if clearErr {
			item.v.ImportError = nil
		}
		out = append(out, item.v)
	}
	return out
}

func buildResourceRecommendations(l meta.ImportList) [][]string {
	resourceToAzureIdMapping := mapping.ProviderResourceMapping
	azureIdToResourcesMapping := map[string][]string{}
	for k, v := range resourceToAzureIdMapping {
		resources, ok := azureIdToResourcesMapping[v]
		if !ok {
			resources = []string{}
		}
		resources = append(resources, k)
		azureIdToResourcesMapping[v] = resources
	}
	azureIdPatternToResourcesMapping := map[*regexp.Regexp][]string{}
	for path, resources := range azureIdToResourcesMapping {
		p := regexp.MustCompile("^" + strings.ReplaceAll(path, "{}", "[^/]+") + "$")
		azureIdPatternToResourcesMapping[p] = resources
	}

	recommendations := [][]string{}
	for _, item := range l {
		var recommendation []string
		for pattern, resources := range azureIdPatternToResourcesMapping {
			if pattern.MatchString(strings.ToUpper(item.ResourceID)) {
				recommendation = resources
				break
			}
		}
		recommendations = append(recommendations, recommendation)
	}
	return recommendations
}
