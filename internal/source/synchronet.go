package source

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const synchronetListURL = "https://synchro.net/sbbslist.html"

var synchronetServiceRE = regexp.MustCompile(`^([^\s]+)\s+\((telnet|ssh)\)`) // passive protocols supported by BBSIntel

type Synchronet struct {
	Client *http.Client
}

func (a *Synchronet) Name() string { return "synchronet" }

func (a *Synchronet) Fetch(ctx context.Context) ([]Entry, error) {
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, synchronetListURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "BBSIntel/0.1 (+https://github.com/Ploos-AS/BBSIntel)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", synchronetListURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %s", synchronetListURL, resp.Status)
	}
	entries, err := parseSynchronet(resp.Body, synchronetListURL)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", synchronetListURL, err)
	}
	return entries, nil
}

func parseSynchronet(r interface{ Read([]byte) (int, error) }, pageURL string) ([]Entry, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, row := range synchronetRows(doc) {
		cells := directCells(row)
		if len(cells) < 5 {
			continue
		}
		identityLines := nodeTextLines(cells[0])
		if len(identityLines) == 0 || strings.EqualFold(identityLines[0], "BBS") {
			continue
		}
		name := identityLines[0]
		description := ""
		if len(identityLines) > 1 {
			description = strings.Join(identityLines[1:], " ")
		}

		type endpointSpec struct {
			protocol string
			hostname string
			port     int
		}
		var endpoints []endpointSpec
		for _, line := range nodeTextLines(cells[4]) {
			m := synchronetServiceRE.FindStringSubmatch(strings.TrimSpace(line))
			if len(m) != 3 {
				continue
			}
			protocol := strings.ToLower(m[2])
			defaultPort := 23
			if protocol == "ssh" {
				defaultPort = 22
			}
			host, port := splitEndpoint(m[1], defaultPort)
			if host == "" {
				continue
			}
			endpoints = append(endpoints, endpointSpec{protocol: protocol, hostname: host, port: port})
		}
		if len(endpoints) == 0 {
			continue
		}

		// One stable source key is shared by all terminal endpoints in this row so
		// ingest can attach Telnet and SSH services to the same BBS identity.
		key := normalizeSourceKey(name) + "|" + strings.ToLower(endpoints[0].hostname)
		for _, ep := range endpoints {
			out = append(out, Entry{
				Source:      "synchronet",
				SourceKey:   key,
				SourceURL:   pageURL,
				Name:        name,
				Software:    "Synchronet",
				Description: description,
				Protocol:    ep.protocol,
				Hostname:    ep.hostname,
				Port:        ep.port,
			})
		}
	}
	return out, nil
}

func synchronetRows(n *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode && x.Data == "tr" {
			out = append(out, x)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func directCells(row *html.Node) []*html.Node {
	var out []*html.Node
	for c := row.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
			out = append(out, c)
		}
	}
	return out
}

// nodeTextLines preserves visual line boundaries created by <br> while joining
// adjacent text nodes (for example an <a> hostname followed by " (telnet)").
func nodeTextLines(n *html.Node) []string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode && x.Data == "br" {
			b.WriteByte('\n')
			return
		}
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	var out []string
	for _, part := range strings.Split(b.String(), "\n") {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, strings.Join(strings.Fields(s), " "))
		}
	}
	return out
}

func normalizeSourceKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}
