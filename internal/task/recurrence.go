package task

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
)

var errUnsupportedRule = errors.New("repeat rule too complex to advance here, complete it in Tasks.org")

var weekdays = map[string]time.Weekday{"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday, "TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday}

type rule struct {
	frequency string
	interval  int
	count     int
	until     *time.Time // exclusive bound
	byDay     []time.Weekday
}

// Complete marks the task done. A recurring task instead moves to its next occurrence, like Tasks.org does.
func Complete(task Task) (Task, error) {
	base := task.Due
	if base == nil {
		base = task.Start
	}
	if task.Recurrence == "" || base == nil {
		task.Status = Completed
		return task, nil
	}
	parsed, err := parseRule(task.Recurrence)
	if err != nil {
		return task, err
	}
	months, days := parsed.step(base.Time.Local())
	next := base.Time.Local().AddDate(0, months, days)
	if parsed.count == 1 || (parsed.until != nil && !next.Before(*parsed.until)) {
		task.Status = Completed
		return task, nil
	}
	if parsed.count > 1 {
		task.Recurrence = setRulePart(task.Recurrence, "COUNT", strconv.Itoa(parsed.count-1))
	}
	task.Due = shift(task.Due, months, days)
	task.Start = shift(task.Start, months, days)
	task.Status = NeedsAction
	return task, nil
}

func ExpandRule(text string) string {
	text = strings.ToUpper(strings.TrimSpace(text))
	switch text {
	case "DAILY", "WEEKLY", "MONTHLY", "YEARLY":
		return "FREQ=" + text
	}
	return text
}

// ponytail: only FREQ/INTERVAL/COUNT/UNTIL/BYDAY (plain weekdays) are advanced; other rules are refused rather than guessed
func parseRule(value string) (rule, error) {
	parsed := rule{interval: 1}
	for _, part := range strings.Split(strings.ToUpper(value), ";") {
		key, content, _ := strings.Cut(part, "=")
		var err error
		switch key {
		case "FREQ":
			parsed.frequency = content
		case "INTERVAL":
			parsed.interval, err = strconv.Atoi(content)
		case "COUNT":
			parsed.count, err = strconv.Atoi(content)
		case "UNTIL":
			parsed.until, err = parseUntil(content)
		case "BYDAY":
			for _, code := range strings.Split(content, ",") {
				weekday, known := weekdays[code]
				if !known {
					return parsed, errUnsupportedRule
				}
				parsed.byDay = append(parsed.byDay, weekday)
			}
		case "WKST", "":
		default:
			return parsed, errUnsupportedRule
		}
		if err != nil {
			return parsed, errors.New("invalid repeat rule: " + part)
		}
	}
	supported := slices.Contains([]string{"DAILY", "WEEKLY", "MONTHLY", "YEARLY"}, parsed.frequency)
	if !supported || parsed.interval < 1 || (len(parsed.byDay) > 0 && parsed.frequency != "WEEKLY" && parsed.frequency != "DAILY") {
		return parsed, errUnsupportedRule
	}
	return parsed, nil
}

func parseUntil(value string) (*time.Time, error) {
	if until, err := time.Parse("20060102T150405Z", value); err == nil {
		return &until, nil
	}
	until, err := time.ParseInLocation("20060102", value, time.Local)
	if err != nil {
		return nil, err
	}
	until = until.AddDate(0, 0, 1)
	return &until, nil
}

func (recurrence rule) step(from time.Time) (months, days int) {
	switch {
	case recurrence.frequency == "MONTHLY":
		return recurrence.interval, 0
	case recurrence.frequency == "YEARLY":
		return 12 * recurrence.interval, 0
	case len(recurrence.byDay) == 0 && recurrence.frequency == "WEEKLY":
		return 0, 7 * recurrence.interval
	case len(recurrence.byDay) == 0:
		return 0, recurrence.interval
	}
	// weeks start on Monday (the RRULE default WKST), which decides when INTERVAL skips ahead
	mondayIndex := (int(from.Weekday()) + 6) % 7
	for days = 1; days <= 7; days++ {
		if !slices.Contains(recurrence.byDay, from.AddDate(0, 0, days).Weekday()) {
			continue
		}
		if recurrence.frequency == "WEEKLY" && mondayIndex+days >= 7 {
			days += 7 * (recurrence.interval - 1)
		}
		return 0, days
	}
	return 0, 7
}

func setRulePart(value, key, content string) string {
	parts := strings.Split(value, ";")
	for index, part := range parts {
		if strings.HasPrefix(strings.ToUpper(part), key+"=") {
			parts[index] = key + "=" + content
		}
	}
	return strings.Join(parts, ";")
}

func shift(value *Date, months, days int) *Date {
	if value == nil {
		return nil
	}
	return &Date{Time: value.Time.Local().AddDate(0, months, days), AllDay: value.AllDay}
}
