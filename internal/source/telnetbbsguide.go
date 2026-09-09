package source

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const telnetBBSGuideBase = "https://www.telnetbbsguide.com"

type TelnetBBSGuide struct {
	Client *http.Client
}

func (a *TelnetBBSGuide) Name() string { return "telnetbbsguide" }

func (a *TelnetBBSGuide) Fetch(ctx context.Context) ([]Entry, error) {
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	var out []Entry
	for _, letter := range "abcdefghijklmnopqrstuvwxyz" {
		u := fmt.Sprintf("%s/bbs/connection/telnet/list/detail/?ap=%c", telnetBBSGuideBase, letter)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "BBSIntel/0.1 (+https://github.com/Ploos-AS/BBSIntel)")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", u, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("fetch %s: HTTP %s", u, resp.Status)
		}
		entries, err := parseTelnetBBSGuide(resp.Body, u)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", u, err)
		}
		out = append(out, entries...)
	}
	return out, nil
}

func parseTelnetBBSGuide(r interface{ Read([]byte) (int, error) }, pageURL string) ([]Entry, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	lines := textLines(doc)
	var out []Entry
	for i := 0; i < len(lines); i++ {
		protocol, label, defaultPort, ok := connectionLabel(lines[i])
		if !ok {
			continue
		}
		endpoint := strings.TrimSpace(strings.TrimPrefix(lines[i], label))
		if endpoint == "" && i+1 < len(lines) {
			i++
			endpoint = strings.TrimSpace(lines[i])
		}
		if endpoint == "" {
			continue
		}
		name := previousName(lines, i)
		software := ""
		for j := i + 1; j < len(lines) && j < i+8; j++ {
			if strings.HasPrefix(lines[j], "Software:") {
				software = strings.TrimSpace(strings.TrimPrefix(lines[j], "Software:"))
				break
			}
		}
		host, port := splitEndpoint(endpoint, defaultPort)
		if host == "" {
			continue
		}
		key := strings.ToLower(host) + ":" + strconv.Itoa(port)
		if protocol != "telnet" {
			key = protocol + ":" + key
		}
		out = append(out, Entry{
			Source:    "telnetbbsguide",
			SourceKey: key,
			SourceURL: pageURL,
			Name:      name,
			Software:  software,
			Protocol:  protocol,
			Hostname:  host,
			Port:      port,
		})
	}
	return out, nil
}

func connectionLabel(line string) (string, string, int, bool) {
	switch {
	case strings.HasPrefix(line, "Telnet:"):
		return "telnet", "Telnet:", 23, true
	case strings.HasPrefix(line, "SSH:"):
		return "ssh", "SSH:", 22, true
	default:
		return "", "", 0, false
	}
}

func textLines(n *html.Node) []string {
	var raw []string
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			for _, s := range strings.Split(x.Data, "\n") {
				if s = strings.TrimSpace(s); s != "" {
					raw = append(raw, s)
				}
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return raw
}

func previousName(lines []string, idx int) string {
	for i := idx - 1; i >= 0 && i >= idx-10; i-- {
		s := strings.TrimSpace(lines[i])
		if s == "Image" || strings.Contains(s, ":") || s == "MORE..." || looksLikeEndpoint(s) {
			continue
		}
		if len(s) > 1 {
			return s
		}
	}
	return "Unknown BBS"
}

func looksLikeEndpoint(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \t") {
		return false
	}
	if strings.HasPrefix(s, "telnet://") || strings.HasPrefix(s, "ssh://") {
		return true
	}
	return strings.Contains(s, ".") || strings.HasPrefix(s, "[")
}

func splitEndpoint(s string, defaultPort int) (string, int) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s, "telnet://"), "ssh://"))
	if u, err := url.Parse("//" + s); err == nil {
		h := u.Hostname()
		p := defaultPort
		if ps := u.Port(); ps != "" {
			if n, err := strconv.Atoi(ps); err == nil {
				p = n
			}
		}
		return h, p
	}
	return s, defaultPort
}
