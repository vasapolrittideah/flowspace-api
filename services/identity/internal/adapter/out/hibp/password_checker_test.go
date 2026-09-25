package hibp_test

import (
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/hibp"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPasswordCheckerUsesPaddedRange(t *testing.T) {
	password := "correct horse battery staple"
	if !crypto.SHA1.Available() {
		t.Fatal("SHA-1 implementation unavailable")
	}
	hasher := crypto.SHA1.New()
	_, _ = hasher.Write([]byte(password))
	full := strings.ToUpper(hex.EncodeToString(hasher.Sum(nil)))
	client := &http.Client{Transport: roundTrip(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.pwnedpasswords.com/range/"+full[:5] || request.Header.Get("Add-Padding") != "true" {
			t.Fatalf("unsafe breach request: %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(full[5:] + ":1\r\n"))}, nil
	})}
	blocked, err := hibp.NewPasswordChecker(client).Compromised(context.Background(), password)
	if err != nil || !blocked {
		t.Fatalf("blocked = %t, error = %v", blocked, err)
	}
}

func TestPasswordCheckerFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       int
		body         string
		transportErr error
	}{
		{"service unavailable", http.StatusServiceUnavailable, "", nil},
		{"network failure", 0, "", errors.New("offline")},
		{"invalid response", http.StatusOK, "invalid", nil},
		{"oversized response", http.StatusOK, strings.Repeat("A", (2<<20)+1), nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
				if test.transportErr != nil {
					return nil, test.transportErr
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})}
			if blocked, err := hibp.NewPasswordChecker(client).Compromised(context.Background(), "correct horse battery staple"); err == nil || blocked {
				t.Fatalf("blocked = %t, error = %v", blocked, err)
			}
		})
	}
}
