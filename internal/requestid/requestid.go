// Package requestid validates and generates request IDs.
package requestid

import (
	"context"
	"crypto/rand"

	"google.golang.org/grpc/metadata"
)

// Valid reports whether value is a request ID with 1 to 128 allowed ASCII bytes.
func Valid(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

// ValidOrNew keeps a valid value or generates a new request ID.
func ValidOrNew(value string) string {
	if Valid(value) {
		return value
	}
	return rand.Text()
}

// FromIncoming returns the x-request-id gRPC metadata of ctx when it holds
// exactly one valid request ID. Otherwise it returns an empty string.
func FromIncoming(ctx context.Context) string {
	values := metadata.ValueFromIncomingContext(ctx, "x-request-id")
	if len(values) == 1 && Valid(values[0]) {
		return values[0]
	}
	return ""
}
