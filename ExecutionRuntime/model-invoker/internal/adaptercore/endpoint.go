package adaptercore

import (
	"net"
	"net/url"
	"strings"
)

// NormalizeEndpoint removes only a trailing path slash while preserving the
// remaining URL components. Public request/config validation owns URL safety.
func NormalizeEndpoint(value string) string {
	if value == "" {
		return ""
	}
	u, err := url.Parse(value)
	if err != nil {
		return strings.TrimRight(value, "/")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}

// EffectiveEndpoint reports the request override when present, otherwise the
// adapter's configured endpoint.
func EffectiveEndpoint(requested, configured string) string {
	if requested != "" {
		return NormalizeEndpoint(requested)
	}
	return NormalizeEndpoint(configured)
}

// IsLoopbackHost reports whether host is localhost or a loopback IP address.
func IsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
