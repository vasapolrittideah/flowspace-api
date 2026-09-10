package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewWorkspace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantErr  bool
	}{
		{name: "trims name", input: "  Platform  ", wantName: "Platform"},
		{name: "accepts 100 characters", input: strings.Repeat("A", 100), wantName: strings.Repeat("A", 100)},
		{name: "rejects blank name", input: " \t", wantErr: true},
		{name: "rejects more than 100 characters", input: strings.Repeat("A", 101), wantErr: true},
		{name: "rejects invalid UTF-8", input: string([]byte{0xff}), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace, err := NewWorkspace(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("error = %v, want ErrInvalidArgument", err)
				}
				var invalid *InvalidArgumentError
				if !errors.As(err, &invalid) || invalid.Field != "name" {
					t.Fatalf("error = %v, want name field error", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if workspace.Name != tt.wantName {
				t.Fatalf("name = %q, want %q", workspace.Name, tt.wantName)
			}
		})
	}
}
