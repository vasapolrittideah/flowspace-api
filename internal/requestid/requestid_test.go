package requestid

import (
	"strings"
	"testing"
)

func TestValidOrNew(t *testing.T) {
	for _, value := range []string{"a", "Request.ID_3-", strings.Repeat("x", 128)} {
		if !Valid(value) || ValidOrNew(value) != value {
			t.Fatalf("valid request ID %q was replaced", value)
		}
	}
	for _, value := range []string{"", "has space", "has/slash", "คำขอ", strings.Repeat("x", 129)} {
		if got := ValidOrNew(value); got == value || !Valid(got) {
			t.Fatalf("invalid request ID %q was not replaced with a valid ID: %q", value, got)
		}
	}
}
