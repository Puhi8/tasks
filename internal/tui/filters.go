package tui

import (
	"strings"
	"time"

	"github.com/puhi8/tasks/internal/task"
)

// filter is a parsed query: plain words search title and notes, #word matches a tag, key:value words filter.
type filter struct {
	words    []string
	status   string
	tags     []string
	priority string
	due      string
	list     string
}

func parseQuery(query string) filter {
	var parsed filter
	for _, word := range strings.Fields(strings.ToLower(query)) {
		key, value, found := strings.Cut(word, ":")
		switch {
		case strings.HasPrefix(word, "#") && len(word) > 1:
			parsed.tags = append(parsed.tags, word[1:])
		case !found || value == "":
			parsed.words = append(parsed.words, word)
		case key == "status":
			parsed.status = value
		case key == "tag":
			parsed.tags = append(parsed.tags, value)
		case key == "priority":
			parsed.priority = value
		case key == "due":
			parsed.due = value
		case key == "list":
			parsed.list = value
		default:
			parsed.words = append(parsed.words, word)
		}
	}
	return parsed
}

func (criteria filter) match(todo task.Task, calendarName string, now time.Time) bool {
	switch {
	case criteria.status == "open" && todo.Done(),
		(criteria.status == "completed" || criteria.status == "done") && !todo.Done(),
		criteria.priority != "" && task.PriorityLabel(todo.Priority) != criteria.priority,
		criteria.list != "" && !strings.Contains(strings.ToLower(calendarName), criteria.list),
		criteria.due == "overdue" && !overdue(todo, now),
		criteria.due == "today" && (todo.Due == nil || daysBetween(now, todo.Due.Time) != 0),
		criteria.due == "soon" && (todo.Due == nil || todo.Done() || daysBetween(now, todo.Due.Time) > 7),
		criteria.due == "none" && todo.Due != nil:
		return false
	}
	for _, tag := range criteria.tags {
		if !hasTag(todo, tag) {
			return false
		}
	}
	text := strings.ToLower(todo.Summary + "\n" + todo.Description)
	for _, word := range criteria.words {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

// tagToken writes a tag as one query word; spaces become underscores.
func tagToken(tag string) string { return strings.ReplaceAll(strings.ToLower(tag), " ", "_") }

func hasTag(todo task.Task, token string) bool {
	for _, category := range todo.Categories {
		if tagToken(category) == token {
			return true
		}
	}
	return false
}

// setQueryToken only removes the matching words when value is empty.
func setQueryToken(query, prefix, value string) string {
	var words []string
	for _, word := range strings.Fields(query) {
		if !strings.HasPrefix(strings.ToLower(word), prefix) {
			words = append(words, word)
		}
	}
	if value != "" {
		words = append(words, prefix+value)
	}
	return strings.Join(words, " ")
}

func overdue(todo task.Task, now time.Time) bool {
	if todo.Due == nil || todo.Done() {
		return false
	}
	if todo.Due.AllDay {
		return daysBetween(now, todo.Due.Time) < 0
	}
	return todo.Due.Time.Before(now)
}

// daysBetween compares calendar dates in local time, so clock times and DST shifts do not count.
func daysBetween(first, second time.Time) int {
	dayOf := func(value time.Time) time.Time {
		local := value.Local()
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	}
	return int(dayOf(second).Sub(dayOf(first)).Hours() / 24)
}

func dateLabel(value *task.Date, now time.Time) string {
	if value == nil {
		return ""
	}
	local := value.Time.Local()
	clock := ""
	if !value.AllDay {
		clock = " " + local.Format("15:04")
	}
	days := daysBetween(now, local)
	switch {
	case days == 0 && clock != "":
		return strings.TrimSpace(clock)
	case days == 0:
		return "today"
	case days == 1:
		return "tomorrow" + clock
	case days == -1:
		return "yesterday" + clock
	case days > 1 && days < 7:
		return local.Format("Mon") + clock
	case local.Year() == now.Year():
		return local.Format("Jan 2") + clock
	}
	return local.Format(time.DateOnly)
}
