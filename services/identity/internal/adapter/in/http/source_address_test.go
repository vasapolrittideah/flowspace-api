package http_test

import (
	"net/http"
	"testing"

	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
)

func TestSourceAddress(t *testing.T) {
	trusted, err := identityhttp.TrustedProxies([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, peer, forwarded, other, want string
	}{
		{"direct", "192.0.2.1:1234", "198.51.100.1", "", "192.0.2.1"},
		{"trusted chain", "10.0.0.2:1234", "192.0.2.1, 10.0.0.1", "", "192.0.2.1"},
		{"rightmost untrusted", "10.0.0.2:1234", "192.0.2.1, 198.51.100.2, 10.0.0.1", "", "198.51.100.2"},
		{"missing chain", "10.0.0.2:1234", "", "", ""},
		{"malformed chain", "10.0.0.2:1234", "192.0.2.1, bad", "", ""},
		{"forbidden header", "192.0.2.1:1234", "", "Forwarded", ""},
		{"real ip header", "192.0.2.1:1234", "", "X-Real-IP", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			header := http.Header{}
			if test.forwarded != "" {
				header.Set("X-Forwarded-For", test.forwarded)
			}
			if test.other != "" {
				header.Set(test.other, "192.0.2.100")
			}
			got, err := identityhttp.SourceAddress(test.peer, header, trusted)
			if (err != nil) != (test.want == "") || got != test.want {
				t.Fatalf("source = %q, %v; want %q", got, err, test.want)
			}
		})
	}
	if _, err := identityhttp.TrustedProxies([]string{"0.0.0.0/0"}); err == nil {
		t.Fatal("wildcard trusted proxy accepted")
	}
}
