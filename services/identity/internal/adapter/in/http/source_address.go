package http

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

var ErrInvalidSourceAddress = errors.New("invalid source address")

func TrustedProxies(cidrs []string) ([]netip.Prefix, error) {
	proxies := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil || prefix.Bits() == 0 || prefix.Addr().Zone() != "" {
			return nil, ErrInvalidSourceAddress
		}
		proxies = append(proxies, prefix.Masked())
	}
	return proxies, nil
}

func SourceAddress(remote string, headers http.Header, trusted []netip.Prefix) (string, error) {
	if len(headers.Values("Forwarded")) != 0 || len(headers.Values("X-Real-IP")) != 0 {
		return "", ErrInvalidSourceAddress
	}
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return "", ErrInvalidSourceAddress
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || peer.Zone() != "" {
		return "", ErrInvalidSourceAddress
	}
	peer = peer.Unmap()
	values := headers.Values("X-Forwarded-For")
	if len(values) > 1 {
		return "", ErrInvalidSourceAddress
	}
	chain := make([]netip.Addr, 0)
	if len(values) == 1 {
		for raw := range strings.SplitSeq(values[0], ",") {
			address, parseErr := netip.ParseAddr(strings.TrimSpace(raw))
			if parseErr != nil || address.Zone() != "" {
				return "", ErrInvalidSourceAddress
			}
			chain = append(chain, address.Unmap())
		}
	}
	if !isTrusted(peer, trusted) {
		return peer.String(), nil
	}
	if len(chain) == 0 {
		return "", ErrInvalidSourceAddress
	}
	for _, address := range slices.Backward(chain) {
		if !isTrusted(address, trusted) {
			return address.String(), nil
		}
	}
	return "", ErrInvalidSourceAddress
}

func isTrusted(address netip.Addr, proxies []netip.Prefix) bool {
	for _, prefix := range proxies {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
