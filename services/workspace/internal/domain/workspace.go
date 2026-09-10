package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

const maxWorkspaceNameRunes = 100

type Workspace struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

func NewWorkspace(name string) (Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Workspace{}, &InvalidArgumentError{Field: "name", Reason: "is required"}
	}
	if !utf8.ValidString(name) {
		return Workspace{}, &InvalidArgumentError{Field: "name", Reason: "must be valid UTF-8"}
	}
	if utf8.RuneCountInString(name) > maxWorkspaceNameRunes {
		return Workspace{}, &InvalidArgumentError{Field: "name", Reason: "must be at most 100 characters"}
	}
	return Workspace{Name: name}, nil
}
