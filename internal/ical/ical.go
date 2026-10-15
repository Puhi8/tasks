package ical

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Param struct {
	Name  string
	Value string // kept raw (quotes, comma lists) so unknown parameters are written back unchanged
}

type Property struct {
	Name   string
	Params []Param
	Value  string // kept raw (still escaped) so unknown properties are written back unchanged
}

type Component struct {
	Name       string
	Properties []*Property
	Children   []*Component
}

func Parse(data string) (*Component, error) {
	var root *Component
	var stack []*Component
	for _, line := range Unfold(data) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		property, err := parseLine(line)
		if err != nil {
			return nil, err
		}
		switch property.Name {
		case "BEGIN":
			component := &Component{Name: strings.ToUpper(property.Value)}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, component)
			} else if root != nil {
				return nil, errors.New("more than one top-level component")
			} else {
				root = component
			}
			stack = append(stack, component)
		case "END":
			if len(stack) == 0 || stack[len(stack)-1].Name != strings.ToUpper(property.Value) {
				return nil, fmt.Errorf("unexpected END:%s", property.Value)
			}
			stack = stack[:len(stack)-1]
		default:
			if len(stack) == 0 {
				return nil, fmt.Errorf("property %s outside of a component", property.Name)
			}
			current := stack[len(stack)-1]
			current.Properties = append(current.Properties, property)
		}
	}
	if root == nil {
		return nil, errors.New("no component found")
	}
	if len(stack) > 0 {
		return nil, fmt.Errorf("missing END:%s", stack[len(stack)-1].Name)
	}
	return root, nil
}

func Unfold(data string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		if len(lines) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			lines[len(lines)-1] += line[1:]
		} else {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseLine(line string) (*Property, error) {
	colon := indexUnquoted(line, ':')
	if colon < 0 {
		return nil, fmt.Errorf("invalid content line %q", line)
	}
	parts := splitUnquoted(line[:colon], ';')
	property := &Property{Name: strings.ToUpper(parts[0]), Value: line[colon+1:]}
	if property.Name == "" {
		return nil, fmt.Errorf("invalid content line %q", line)
	}
	for _, part := range parts[1:] {
		name, value, found := strings.Cut(part, "=")
		if !found {
			return nil, fmt.Errorf("invalid parameter %q", part)
		}
		property.Params = append(property.Params, Param{Name: strings.ToUpper(name), Value: value})
	}
	return property, nil
}

func indexUnquoted(text string, separator byte) int {
	quoted := false
	for index := 0; index < len(text); index++ {
		if text[index] == '"' {
			quoted = !quoted
		} else if text[index] == separator && !quoted {
			return index
		}
	}
	return -1
}

func splitUnquoted(text string, separator byte) []string {
	var parts []string
	for {
		index := indexUnquoted(text, separator)
		if index < 0 {
			return append(parts, text)
		}
		parts = append(parts, text[:index])
		text = text[index+1:]
	}
}

func (component *Component) String() string {
	var builder strings.Builder
	component.write(&builder)
	return builder.String()
}

func (component *Component) write(builder *strings.Builder) {
	writeLine(builder, "BEGIN:"+component.Name)
	for _, property := range component.Properties {
		writeLine(builder, property.String())
	}
	for _, child := range component.Children {
		child.write(builder)
	}
	writeLine(builder, "END:"+component.Name)
}

func (property *Property) String() string {
	var builder strings.Builder
	builder.WriteString(property.Name)
	for _, parameter := range property.Params {
		builder.WriteString(";" + parameter.Name + "=" + parameter.Value)
	}
	builder.WriteString(":" + property.Value)
	return builder.String()
}

// writeLine folds at 75 octets (RFC 5545) without splitting UTF-8 sequences.
func writeLine(builder *strings.Builder, line string) {
	limit := 75
	for len(line) > limit {
		cut := limit
		for cut > 1 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		builder.WriteString(line[:cut] + "\r\n ")
		line = line[cut:]
		limit = 74
	}
	builder.WriteString(line + "\r\n")
}

func (component *Component) Get(name string) *Property {
	if all := component.GetAll(name); len(all) > 0 {
		return all[0]
	}
	return nil
}

func (component *Component) GetAll(name string) []*Property {
	var properties []*Property
	for _, property := range component.Properties {
		if property.Name == name {
			properties = append(properties, property)
		}
	}
	return properties
}

// Upsert appends an empty property when missing, so callers can always set its value.
func (component *Component) Upsert(name string) *Property {
	if property := component.Get(name); property != nil {
		return property
	}
	property := &Property{Name: name}
	component.Properties = append(component.Properties, property)
	return property
}

func (component *Component) Remove(name string) {
	component.RemoveIf(func(property *Property) bool { return property.Name == name })
}

func (component *Component) RemoveIf(match func(*Property) bool) {
	kept := component.Properties[:0]
	for _, property := range component.Properties {
		if !match(property) {
			kept = append(kept, property)
		}
	}
	component.Properties = kept
}

func (property *Property) Param(name string) string {
	for _, parameter := range property.Params {
		if parameter.Name == name {
			return strings.Trim(parameter.Value, `"`)
		}
	}
	return ""
}

// SetParam removes the parameter when value is empty.
func (property *Property) SetParam(name, value string) {
	if strings.ContainsAny(value, ":;,") {
		value = `"` + value + `"`
	}
	for index, parameter := range property.Params {
		if parameter.Name != name {
			continue
		}
		if value == "" {
			property.Params = append(property.Params[:index], property.Params[index+1:]...)
		} else {
			property.Params[index].Value = value
		}
		return
	}
	if value != "" {
		property.Params = append(property.Params, Param{Name: name, Value: value})
	}
}
