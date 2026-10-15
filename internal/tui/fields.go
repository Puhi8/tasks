package tui

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/puhi8/tasks/internal/task"
)

func (form *editor) currentTag() (entered, typing string) {
	value := form.inputs[fieldTags].Value()
	typing = strings.TrimLeft(value[strings.LastIndex(value, ",")+1:], " ")
	return value[:len(value)-len(typing)], typing
}

func (form *editor) refreshTagSuggestions() {
	entered, typing := form.currentTag()
	var suggestions []string
	if typing != "" {
		used := parseTags(entered, nil)
		for _, tag := range form.tags {
			if !containsFold(used, tag) {
				suggestions = append(suggestions, entered+tag+", ")
			}
		}
	}
	form.inputs[fieldTags].SetSuggestions(suggestions)
}

func containsFold(list []string, text string) bool {
	return slices.ContainsFunc(list, func(item string) bool { return strings.EqualFold(item, text) })
}

func (model *Model) tagHints(width int) string {
	form := &model.editor
	indent := strings.Repeat(" ", 14)
	entered, _ := form.currentTag()
	var parts []string
	if matches := form.inputs[fieldTags].MatchedSuggestions(); len(matches) > 0 {
		tags := make([]string, len(matches))
		for index, match := range matches {
			tags[index] = strings.TrimSuffix(strings.TrimPrefix(match, entered), ", ")
		}
		return model.renderChoices(tags, tags, form.inputs[fieldTags].CurrentSuggestionIndex(), indent, width)
	}
	used := parseTags(form.inputs[fieldTags].Value(), nil)
	for _, tag := range form.tags {
		if !containsFold(used, tag) {
			parts = append(parts, model.tagStyle(tag).Render(tag))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return ansi.Truncate(indent+dimStyle.Render("known: ")+strings.Join(parts, " "), width, "…")
}

func formatDateInput(value *task.Date) string {
	if value == nil {
		return ""
	}
	if value.AllDay {
		return value.Time.Format(time.DateOnly)
	}
	return value.Time.Local().Format("2006-01-02 15:04")
}

// parseDateInput keeps the original date untouched when its text was not edited, so nothing is rewritten needlessly.
func parseDateInput(text string, original *task.Date) (*task.Date, error) {
	text = strings.TrimSpace(text)
	if original != nil && text == formatDateInput(original) {
		return original, nil
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	lower := strings.ToLower(text)
	switch lower {
	case "":
		return nil, nil
	case "today":
		return &task.Date{Time: today, AllDay: true}, nil
	case "tomorrow":
		return &task.Date{Time: today.AddDate(0, 0, 1), AllDay: true}, nil
	}
	if days, err := strconv.Atoi(strings.TrimSuffix(lower, "d")); err == nil && strings.HasPrefix(lower, "+") {
		return &task.Date{Time: today.AddDate(0, 0, days), AllDay: true}, nil
	}
	if value, err := time.ParseInLocation("2006-01-02 15:04", text, time.Local); err == nil {
		return &task.Date{Time: value}, nil
	}
	if value, err := time.ParseInLocation(time.DateOnly, text, time.Local); err == nil {
		return &task.Date{Time: value, AllDay: true}, nil
	}
	return nil, errors.New("use YYYY-MM-DD, YYYY-MM-DD HH:MM, today, tomorrow or +N days")
}

func parseTags(text string, original []string) []string {
	if text == strings.Join(original, ", ") {
		return original
	}
	var tags []string
	for _, tag := range strings.Split(text, ",") {
		if tag = strings.TrimSpace(tag); tag != "" && !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	return tags
}
