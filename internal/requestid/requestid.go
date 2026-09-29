// Package requestid validates and generates request IDs.
package requestid

import "crypto/rand"

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
