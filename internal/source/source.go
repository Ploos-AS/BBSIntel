package source

import "context"

type Entry struct {
	Source      string
	SourceKey   string
	SourceURL   string
	Name        string
	Software    string
	Country     string
	Description string
	Protocol    string
	Hostname    string
	Port        int
}

type Adapter interface {
	Name() string
	Fetch(context.Context) ([]Entry, error)
}
