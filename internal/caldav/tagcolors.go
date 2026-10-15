package caldav

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

// VTODO has no place for tag colors, so they live as one JSON dead property on the calendar home, shared by every client of the account.
const tagColorsNamespace = `xmlns:d="DAV:" xmlns:t="urn:x-tasks-tui"`

// TagColors maps tag to "#rrggbb". Discover finds the calendar home they live on, so it has to run first.
func (client *Client) TagColors() (map[string]string, error) {
	colors := map[string]string{}
	responses, err := client.multistatus("PROPFIND", client.home, "0", `<d:propfind `+tagColorsNamespace+`><d:prop><t:tag-colors/></d:prop></d:propfind>`)
	if err != nil {
		return colors, err
	}
	for _, response := range responses {
		if found, ok := response.found(); ok && strings.TrimSpace(found.TagColors) != "" {
			if err := json.Unmarshal([]byte(found.TagColors), &colors); err != nil {
				return colors, fmt.Errorf("tag colors: %w", err)
			}
		}
	}
	return colors, nil
}

func (client *Client) SetTagColors(colors map[string]string) error {
	data, err := json.Marshal(colors)
	if err != nil {
		return err
	}
	var escaped strings.Builder
	xml.EscapeText(&escaped, data)
	responses, err := client.multistatus("PROPPATCH", client.home, "0", `<d:propertyupdate `+tagColorsNamespace+
		`><d:set><d:prop><t:tag-colors>`+escaped.String()+`</t:tag-colors></d:prop></d:set></d:propertyupdate>`)
	if err != nil {
		return err
	}
	for _, response := range responses {
		if _, ok := response.found(); ok {
			return nil
		}
	}
	return errors.New("tag colors: the server refused to store them")
}
