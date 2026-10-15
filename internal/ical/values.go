package ical

import (
	"strings"
	"time"
)

var textEscaper = strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)

func EscapeText(text string) string { return textEscaper.Replace(text) }

// SplitList unescapes a TEXT value; with byComma it also splits on unescaped commas, as CATEGORIES needs.
func SplitList(text string, byComma bool) []string {
	var items []string
	var current strings.Builder
	for index := 0; index < len(text); index++ {
		character := text[index]
		switch {
		case character == '\\' && index+1 < len(text):
			index++
			if text[index] == 'n' || text[index] == 'N' {
				current.WriteByte('\n')
			} else {
				current.WriteByte(text[index])
			}
		case character == ',' && byComma:
			items = append(items, current.String())
			current.Reset()
		default:
			current.WriteByte(character)
		}
	}
	return append(items, current.String())
}

func (property *Property) Text() string { return strings.Join(SplitList(property.Value, false), "") }

// Time falls back to local time for floating times and time zones the system does not know.
func (property *Property) Time() (value time.Time, allDay bool, err error) {
	if len(property.Value) == 8 || strings.EqualFold(property.Param("VALUE"), "DATE") {
		value, err = time.ParseInLocation("20060102", property.Value, time.Local)
		return value, true, err
	}
	if strings.HasSuffix(property.Value, "Z") {
		value, err = time.Parse("20060102T150405Z", property.Value)
		return value, false, err
	}
	location := time.Local
	// ponytail: TZIDs are resolved via the system zone database, not the embedded VTIMEZONE; parse VTIMEZONE if non-Olson TZIDs show up
	if timeZone := property.Param("TZID"); timeZone != "" {
		if zone, loadError := time.LoadLocation(timeZone); loadError == nil {
			location = zone
		}
	}
	value, err = time.ParseInLocation("20060102T150405", property.Value, location)
	return value, false, err
}

// SetTime keeps an existing time zone the system knows, so Tasks.org keeps showing the same zone; otherwise it writes UTC.
func (property *Property) SetTime(value time.Time, allDay bool) {
	property.SetParam("VALUE", "")
	if allDay {
		property.SetParam("TZID", "")
		property.SetParam("VALUE", "DATE")
		property.Value = value.Format("20060102")
		return
	}
	if timeZone := property.Param("TZID"); timeZone != "" {
		if location, err := time.LoadLocation(timeZone); err == nil {
			property.Value = value.In(location).Format("20060102T150405")
			return
		}
		property.SetParam("TZID", "")
	}
	property.Value = value.UTC().Format("20060102T150405Z")
}
