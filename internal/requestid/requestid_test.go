package requestid

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/metadata"
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

func TestFromIncomingKeepsOnlyOneValidID(t *testing.T) {
	for _, test := range []struct {
		pairs []string
		want  string
	}{
		{[]string{"x-request-id", "Request.ID_3-"}, "Request.ID_3-"},
		{[]string{"x-request-id", strings.Repeat("x", 128)}, strings.Repeat("x", 128)},
		{nil, ""},
		{[]string{"x-request-id", ""}, ""},
		{[]string{"x-request-id", "has space"}, ""},
		{[]string{"x-request-id", strings.Repeat("x", 129)}, ""},
		{[]string{"x-request-id", "first", "x-request-id", "second"}, ""},
	} {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(test.pairs...))
		if got := FromIncoming(ctx); got != test.want {
			t.Errorf("FromIncoming(%v) = %q, want %q", test.pairs, got, test.want)
		}
	}
	if got := FromIncoming(context.Background()); got != "" {
		t.Errorf("FromIncoming without metadata = %q", got)
	}
}
