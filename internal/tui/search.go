package tui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (model *Model) updateSearch(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		model.query = ""
		model.refresh()
		fallthrough
	case "enter":
		model.leaveSearch()
		return nil
	}
	var command tea.Cmd
	model.search, command = model.search.Update(key)
	model.search.SetSuggestions(model.tagSuggestions(model.search.Value()))
	model.query = model.search.Value()
	model.refresh()
	return command
}

func (model *Model) tagSuggestions(query string) []string {
	start := strings.LastIndex(query, " ") + 1
	if !strings.HasPrefix(query[start:], "#") {
		return nil
	}
	tags, _ := model.tagCounts(model.calendarPath)
	var suggestions []string
	for _, tag := range tags {
		if suggestion := query[:start] + "#" + tagToken(tag) + " "; !slices.Contains(suggestions, suggestion) {
			suggestions = append(suggestions, suggestion)
		}
	}
	return suggestions
}

func (model *Model) suggestionLine() string {
	matches := model.search.MatchedSuggestions()
	if len(matches) == 0 {
		return ""
	}
	tags, _ := model.tagCounts(model.calendarPath)
	names := map[string]string{}
	for _, tag := range tags {
		names[tagToken(tag)] = tag
	}
	labels := make([]string, len(matches))
	matchedTags := make([]string, len(matches))
	for index, match := range matches {
		words := strings.Fields(match)
		labels[index] = words[len(words)-1]
		matchedTags[index] = names[strings.TrimPrefix(labels[index], "#")]
	}
	return model.renderChoices(labels, matchedTags, model.search.CurrentSuggestionIndex(), " ", model.listWidth())
}

// renderChoices starts a little before the chosen tag so it stays visible in a long list.
func (model *Model) renderChoices(labels, tags []string, current int, indent string, width int) string {
	var parts []string
	for index := max(current-3, 0); index < len(labels); index++ {
		style := model.tagStyle(tags[index])
		if index == current {
			style = style.Inherit(selectedStyle)
		}
		parts = append(parts, style.Render(labels[index]))
	}
	return ansi.Truncate(indent+strings.Join(parts, " ")+dimStyle.Render("   tab complete · ↑/↓ choose"), width, "…")
}
