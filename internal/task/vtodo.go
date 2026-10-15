package task

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/puhi8/tasks/internal/ical"
)

func parseTodo(raw string) (calendar, todo *ical.Component, err error) {
	if calendar, err = ical.Parse(raw); err != nil {
		return nil, nil, err
	}
	if todo = mainTodo(calendar); todo == nil {
		return nil, nil, errors.New("no VTODO found")
	}
	return calendar, todo, nil
}

// mainTodo skips overridden recurrence instances (RECURRENCE-ID) in favour of the master VTODO.
func mainTodo(calendar *ical.Component) *ical.Component {
	var first *ical.Component
	for _, child := range calendar.Children {
		if child.Name != "VTODO" {
			continue
		}
		if child.Get("RECURRENCE-ID") == nil {
			return child
		}
		if first == nil {
			first = child
		}
	}
	return first
}

func read(todo *ical.Component) Task {
	parsed := Task{
		UID:         text(todo, "UID"),
		Summary:     text(todo, "SUMMARY"),
		Description: text(todo, "DESCRIPTION"),
		Status:      Status(strings.ToUpper(strings.TrimSpace(raw(todo, "STATUS")))),
		Location:    text(todo, "LOCATION"),
		Recurrence:  raw(todo, "RRULE"),
		Start:       date(todo, "DTSTART"),
		Due:         date(todo, "DUE"),
		Modified:    date(todo, "LAST-MODIFIED")}
	if parsed.Status == "" && todo.Get("COMPLETED") != nil {
		parsed.Status = Completed
	} else if parsed.Status == "" {
		parsed.Status = NeedsAction
	}
	parsed.Priority, _ = strconv.Atoi(strings.TrimSpace(raw(todo, "PRIORITY")))
	for _, property := range todo.GetAll("CATEGORIES") {
		for _, category := range ical.SplitList(property.Value, true) {
			if category = strings.TrimSpace(category); category != "" {
				parsed.Categories = append(parsed.Categories, category)
			}
		}
	}
	for _, property := range todo.Properties {
		if isParentRelation(property) {
			parsed.ParentUID = property.Text()
			break
		}
	}
	if order, err := strconv.ParseInt(strings.TrimSpace(raw(todo, "X-APPLE-SORT-ORDER")), 10, 64); err == nil {
		parsed.SortOrder = &order
	}
	return parsed
}

func patch(todo *ical.Component, old, edited Task, now time.Time) bool {
	changed := false
	// set replaces every copy of a property with one holding value (keeping the first copy's parameters); "" removes it
	set := func(name string, differs bool, value string) {
		if !differs {
			return
		}
		changed = true
		kept := todo.Get(name)
		todo.Remove(name)
		if value != "" {
			if kept == nil {
				kept = &ical.Property{Name: name}
			}
			kept.Value = value
			todo.Properties = append(todo.Properties, kept)
		}
	}
	setDate := func(name string, oldValue, newValue *Date) {
		if sameDate(oldValue, newValue) {
			return
		}
		changed = true
		if newValue == nil {
			todo.Remove(name)
		} else {
			todo.Upsert(name).SetTime(newValue.Time, newValue.AllDay)
		}
	}

	escaped := make([]string, len(edited.Categories))
	for index, category := range edited.Categories {
		escaped[index] = ical.EscapeText(category)
	}
	priority, sortOrder := "", ""
	if edited.Priority != 0 {
		priority = strconv.Itoa(edited.Priority)
	}
	if edited.SortOrder != nil {
		sortOrder = strconv.FormatInt(*edited.SortOrder, 10)
	}
	set("SUMMARY", edited.Summary != old.Summary, ical.EscapeText(edited.Summary))
	set("DESCRIPTION", edited.Description != old.Description, ical.EscapeText(edited.Description))
	set("LOCATION", edited.Location != old.Location, ical.EscapeText(edited.Location))
	set("PRIORITY", edited.Priority != old.Priority, priority)
	set("CATEGORIES", !slices.Equal(edited.Categories, old.Categories), strings.Join(escaped, ","))
	set("X-APPLE-SORT-ORDER", !sameNumber(edited.SortOrder, old.SortOrder), sortOrder)
	set("RRULE", edited.Recurrence != old.Recurrence, edited.Recurrence)
	setDate("DTSTART", old.Start, edited.Start)
	setDate("DUE", old.Due, edited.Due)

	if edited.Status != old.Status {
		changed = true
		todo.Upsert("STATUS").Value = string(edited.Status)
		percent := todo.Get("PERCENT-COMPLETE")
		if edited.Status == Completed {
			if todo.Get("COMPLETED") == nil {
				stamp(todo, "COMPLETED", now)
			}
			if percent != nil {
				percent.Value = "100"
			}
		} else {
			todo.Remove("COMPLETED")
			if percent != nil && percent.Value == "100" {
				todo.Remove("PERCENT-COMPLETE")
			}
		}
	}
	if edited.ParentUID != old.ParentUID {
		changed = true
		todo.RemoveIf(isParentRelation)
		if edited.ParentUID != "" {
			relation := &ical.Property{Name: "RELATED-TO", Params: []ical.Param{{Name: "RELTYPE", Value: "PARENT"}}, Value: ical.EscapeText(edited.ParentUID)}
			todo.Properties = append(todo.Properties, relation)
		}
	}

	if changed {
		stamp(todo, "LAST-MODIFIED", now)
		stamp(todo, "DTSTAMP", now)
	}
	return changed
}

func isParentRelation(property *ical.Property) bool {
	relation := strings.ToUpper(property.Param("RELTYPE"))
	return property.Name == "RELATED-TO" && (relation == "" || relation == "PARENT")
}

func stamp(todo *ical.Component, name string, now time.Time) {
	property := todo.Upsert(name)
	property.Params = nil
	property.Value = now.UTC().Format("20060102T150405Z")
}

func raw(todo *ical.Component, name string) string {
	if property := todo.Get(name); property != nil {
		return property.Value
	}
	return ""
}

func text(todo *ical.Component, name string) string {
	if property := todo.Get(name); property != nil {
		return property.Text()
	}
	return ""
}

func date(todo *ical.Component, name string) *Date {
	property := todo.Get(name)
	if property == nil {
		return nil
	}
	value, allDay, err := property.Time()
	if err != nil {
		return nil
	}
	return &Date{Time: value, AllDay: allDay}
}

func sameDate(first, second *Date) bool {
	if first == nil || second == nil {
		return first == second
	}
	if first.AllDay || second.AllDay {
		return first.AllDay == second.AllDay && first.Time.Format(time.DateOnly) == second.Time.Format(time.DateOnly)
	}
	return first.Time.Equal(second.Time)
}

func sameNumber(first, second *int64) bool {
	return first == second || (first != nil && second != nil && *first == *second)
}
