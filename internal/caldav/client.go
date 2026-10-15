package caldav

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrConflict means an If-Match / If-None-Match precondition failed: the remote copy changed.
	ErrConflict = errors.New("remote task changed")
	ErrNotFound = errors.New("remote task not found")
)

type Client struct {
	base     *url.URL
	home     string // calendar home, known after Discover
	username string
	password string
	http     *http.Client
}

func New(baseURL, username, password string) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, fmt.Errorf("server url must start with http:// or https://, got %q", baseURL)
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	// redirects are reported instead of followed: Go would resend PROPFIND and REPORT as GET
	noRedirects := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: base, username: username, password: password, http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: noRedirects}}, nil
}

type multistatus struct {
	Responses []response `xml:"DAV: response"`
}

type response struct {
	Href     string           `xml:"DAV: href"`
	Statuses []propertyStatus `xml:"DAV: propstat"`
}

type propertyStatus struct {
	Properties properties `xml:"DAV: prop"`
	Status     string     `xml:"DAV: status"`
}

type href struct {
	Href string `xml:"DAV: href"`
}

type properties struct {
	ETag                 string `xml:"DAV: getetag"`
	DisplayName          string `xml:"DAV: displayname"`
	CurrentUserPrincipal href   `xml:"DAV: current-user-principal"`
	CalendarHomeSet      href   `xml:"urn:ietf:params:xml:ns:caldav calendar-home-set"`
	CalendarData         string `xml:"urn:ietf:params:xml:ns:caldav calendar-data"`
	CTag                 string `xml:"http://calendarserver.org/ns/ getctag"`
	TagColors            string `xml:"urn:x-tasks-tui tag-colors"`
	ResourceType         struct {
		Calendar *struct{} `xml:"urn:ietf:params:xml:ns:caldav calendar"`
	} `xml:"DAV: resourcetype"`
	Components struct {
		Items []struct {
			Name string `xml:"name,attr"`
		} `xml:"urn:ietf:params:xml:ns:caldav comp"`
	} `xml:"urn:ietf:params:xml:ns:caldav supported-calendar-component-set"`
}

func (entry response) found() (properties, bool) {
	for _, status := range entry.Statuses {
		if strings.Contains(status.Status, " 200 ") {
			return status.Properties, true
		}
	}
	return properties{}, false
}

func (client *Client) request(method, path string, headers map[string]string, body string) (*http.Response, error) {
	target, err := client.base.Parse(path)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(method, target.String(), strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	if client.username != "" || client.password != "" {
		request.SetBasicAuth(client.username, client.password)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	reply, err := client.http.Do(request)
	var connectionError *url.Error
	switch {
	case errors.As(err, &connectionError):
		return nil, fmt.Errorf("cannot reach %s: %w", client.base.Host, connectionError.Err)
	case err != nil:
		return nil, err
	case reply.StatusCode == http.StatusUnauthorized:
		reply.Body.Close()
		return nil, errors.New("the server refused the username or password")
	case reply.StatusCode/100 == 3:
		reply.Body.Close()
		return nil, fmt.Errorf("the server redirects %s to %s, use that address as url in the config", target, reply.Header.Get("Location"))
	}
	return reply, nil
}

func (client *Client) multistatus(method, path, depth, body string) ([]response, error) {
	headers := map[string]string{"Depth": depth, "Content-Type": "application/xml; charset=utf-8"}
	reply, err := client.request(method, path, headers, `<?xml version="1.0" encoding="utf-8"?>`+body)
	if err != nil {
		return nil, err
	}
	defer reply.Body.Close()
	if reply.StatusCode != http.StatusMultiStatus {
		return nil, statusError(method, path, reply)
	}
	var result multistatus
	if err := xml.NewDecoder(reply.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	return result.Responses, nil
}

// hrefPath turns absolute URLs and paths alike into the escaped path used as the key.
func (client *Client) hrefPath(value string) string {
	resolved, err := client.base.Parse(strings.TrimSpace(value))
	if err != nil {
		return value
	}
	return resolved.EscapedPath()
}

func statusError(method, path string, reply *http.Response) error {
	io.Copy(io.Discard, io.LimitReader(reply.Body, 1<<20))
	return fmt.Errorf("%s %s: %s", method, path, reply.Status)
}
