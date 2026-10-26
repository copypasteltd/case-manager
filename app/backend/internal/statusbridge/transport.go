package statusbridge

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

func permittedIP(ip netip.Addr, private bool) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip == netip.MustParseAddr("100.100.100.200") || ip == netip.MustParseAddr("fd00:ec2::254") {
		return false
	}
	return ip.IsGlobalUnicast() && !ip.IsPrivate() || private && (ip.IsPrivate() || ip.IsLoopback())
}

func endpointURL(e Endpoint) (*url.URL, error) {
	u, err := url.Parse(e.URL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" ||
		u.Scheme != "https" && !(u.Scheme == "http" && e.AllowHTTP) || u.RawPath != "" ||
		strings.ContainsAny(u.Path, "\\\x00") || strings.Contains(u.Path, "//") || u.Path != "" && u.Path != "/" && (path.Clean(u.Path) != u.Path || !strings.HasPrefix(u.Path, "/")) ||
		len(e.AllowedIPs) == 0 || len(e.AllowedIPs) > 16 {
		return nil, fault("ENDPOINT_INVALID")
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fault("ENDPOINT_INVALID")
		}
	}
	for _, raw := range e.AllowedIPs {
		ip, err := netip.ParseAddr(raw)
		if err != nil || !permittedIP(ip, e.AllowPrivate) {
			return nil, fault("ENDPOINT_INVALID")
		}
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u, nil
}

type resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

func endpointClient(e Endpoint, duration time.Duration, dns resolver) (*http.Client, *url.URL, error) {
	u, err := endpointURL(e)
	if err != nil {
		return nil, nil, err
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	allowed := map[netip.Addr]bool{}
	for _, raw := range e.AllowedIPs {
		ip, _ := netip.ParseAddr(raw)
		allowed[ip.Unmap()] = true
	}
	if dns == nil {
		dns = net.DefaultResolver
	}
	tr := &http.Transport{Proxy: nil, DisableCompression: true, MaxIdleConns: 2, MaxIdleConnsPerHost: 2,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: duration, ResponseHeaderTimeout: duration,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if network != "tcp" || addr != net.JoinHostPort(u.Hostname(), port) {
			return nil, fault("ADDRESS_DENIED")
		}
		ips, err := dns.LookupNetIP(ctx, "ip", u.Hostname())
		if err != nil || len(ips) == 0 {
			return nil, fault("ADDRESS_DENIED")
		}
		for _, ip := range ips {
			if !allowed[ip.Unmap()] || !permittedIP(ip, e.AllowPrivate) {
				return nil, fault("ADDRESS_DENIED")
			}
		}
		// Pin the checked IP into the actual dial, avoiding a second DNS lookup.
		var conn net.Conn
		for _, ip := range ips {
			conn, err = (&net.Dialer{Timeout: duration}).DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, fault("CONNECTION_FAILED")
	}
	return &http.Client{Transport: tr, Timeout: duration, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, u, nil
}

func responseBody(res *http.Response, limit int64) ([]byte, error) {
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, fault("RESPONSE_INVALID")
	}
	return b, nil
}
