package caldav

import (
	"strings"

	"github.com/puhi8/tasks/internal/task"
)

const propfindStart = `<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" xmlns:cs="http://calendarserver.org/ns/"><d:prop>`

func (client *Client) Discover() ([]task.List, error) {
	principal, err := client.findHref(client.base.EscapedPath(), "<d:current-user-principal/>", func(found properties) string { return found.CurrentUserPrincipal.Href })
	if err != nil {
		return nil, err
	}
	home, err := client.findHref(principal, "<c:calendar-home-set/>", func(found properties) string { return found.CalendarHomeSet.Href })
	if err != nil {
		return nil, err
	}
	client.home = home
	responses, err := client.multistatus("PROPFIND", home, "1", propfindStart+
		"<d:resourcetype/><d:displayname/><cs:getctag/><c:supported-calendar-component-set/></d:prop></d:propfind>")
	if err != nil {
		return nil, err
	}
	var collections []task.List
	for _, response := range responses {
		found, ok := response.found()
		if !ok || found.ResourceType.Calendar == nil || !supportsTasks(found) {
			continue
		}
		path := client.hrefPath(response.Href)
		name := strings.TrimSpace(found.DisplayName)
		if name == "" {
			name = strings.Trim(path[strings.LastIndex(strings.TrimSuffix(path, "/"), "/"):], "/")
		}
		collections = append(collections, task.List{Path: path, Name: name, CTag: found.CTag})
	}
	return collections, nil
}

// findHref falls back to path itself when the server does not report the property.
func (client *Client) findHref(path, property string, pick func(properties) string) (string, error) {
	responses, err := client.multistatus("PROPFIND", path, "0", propfindStart+property+"</d:prop></d:propfind>")
	if err != nil {
		return "", err
	}
	for _, response := range responses {
		if found, ok := response.found(); ok && strings.TrimSpace(pick(found)) != "" {
			return client.hrefPath(pick(found)), nil
		}
	}
	return path, nil
}

func supportsTasks(found properties) bool {
	for _, item := range found.Components.Items {
		if strings.EqualFold(item.Name, "VTODO") {
			return true
		}
	}
	return len(found.Components.Items) == 0
}
