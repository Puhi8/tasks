package caldav

import (
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strings"
)

type RemoteTask struct {
	Path string
	ETag string
	Data string // empty when only the ETag was requested
}

const multigetBatch = 100

func (client *Client) ListTasks(collectionPath string) ([]RemoteTask, error) {
	return client.report(collectionPath, false, `<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`+
		`<d:prop><d:getetag/></d:prop><c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VTODO"/></c:comp-filter></c:filter></c:calendar-query>`)
}

func (client *Client) FetchTasks(collectionPath string, paths []string) ([]RemoteTask, error) {
	var tasks []RemoteTask
	for start := 0; start < len(paths); start += multigetBatch {
		var hrefs strings.Builder
		for _, path := range paths[start:min(start+multigetBatch, len(paths))] {
			hrefs.WriteString("<d:href>")
			xml.EscapeText(&hrefs, []byte(path))
			hrefs.WriteString("</d:href>")
		}
		batch, err := client.report(collectionPath, true, `<c:calendar-multiget xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`+
			`<d:prop><d:getetag/><c:calendar-data/></d:prop>`+hrefs.String()+`</c:calendar-multiget>`)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, batch...)
	}
	return tasks, nil
}

// report lists the tasks a REPORT answers with; withData skips answers that came without the task itself.
func (client *Client) report(collectionPath string, withData bool, body string) ([]RemoteTask, error) {
	responses, err := client.multistatus("REPORT", collectionPath, "1", body)
	if err != nil {
		return nil, err
	}
	var tasks []RemoteTask
	for _, response := range responses {
		found, ok := response.found()
		if path := client.hrefPath(response.Href); ok && path != collectionPath && (!withData || found.CalendarData != "") {
			tasks = append(tasks, RemoteTask{Path: path, ETag: found.ETag, Data: found.CalendarData})
		}
	}
	return tasks, nil
}

// send maps the answers of single-resource requests: 404 is ErrNotFound, 412 (a failed If-Match / If-None-Match) is ErrConflict.
func (client *Client) send(method, path string, headers map[string]string, body string) (data, etag string, err error) {
	reply, err := client.request(method, path, headers, body)
	if err != nil {
		return "", "", err
	}
	defer reply.Body.Close()
	switch {
	case reply.StatusCode == http.StatusNotFound:
		return "", "", ErrNotFound
	case reply.StatusCode == http.StatusPreconditionFailed:
		return "", "", ErrConflict
	case reply.StatusCode/100 != 2:
		return "", "", statusError(method, path, reply)
	}
	content, err := io.ReadAll(reply.Body)
	return string(content), reply.Header.Get("ETag"), err
}

func (client *Client) GetTask(path string) (data, etag string, err error) {
	return client.send(http.MethodGet, path, nil, "")
}

// PutTask uploads a task. An empty etag means "create only" (If-None-Match), otherwise "update only if unchanged" (If-Match).
func (client *Client) PutTask(path, data, etag string) (string, error) {
	headers := map[string]string{"Content-Type": "text/calendar; charset=utf-8", "If-None-Match": "*"}
	if etag != "" {
		headers = map[string]string{"Content-Type": "text/calendar; charset=utf-8", "If-Match": etag}
	}
	// iCalendar requires CRLF; tasks stored as downloaded have LF because XML parsing normalizes line endings
	data = strings.ReplaceAll(strings.ReplaceAll(data, "\r\n", "\n"), "\n", "\r\n")
	_, etag, err := client.send(http.MethodPut, path, headers, data)
	return etag, err
}

// Delete treats a resource that is already gone as deleted; an empty etag deletes unconditionally (whole lists).
func (client *Client) Delete(path, etag string) error {
	headers := map[string]string{"If-Match": etag}
	if etag == "" {
		headers = nil
	}
	if _, _, err := client.send(http.MethodDelete, path, headers, ""); !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}
