package statusbridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fixedDNS struct{ ips []netip.Addr }

func (d *fixedDNS) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return d.ips, nil
}

func TestStatusBridgeTargetControls(t *testing.T) {
	for _, host := range []string{"169.254.169.254", "169.254.170.2", "100.100.100.200", "fd00:ec2::254", "::ffff:169.254.169.254", "0.0.0.0", "224.0.0.1", "::"} {
		t.Run(host, func(t *testing.T) {
			e := Endpoint{URL: "http://example.invalid", AllowedIPs: []string{host}, AllowPrivate: true, AllowHTTP: true}
			if _, err := endpointURL(e); err == nil {
				t.Fatal("metadata or special address accepted")
			}
		})
	}
	for _, raw := range []string{"http://example.invalid", "https://name:secret@example.invalid", "https://example.invalid/?target=x", "https://example.invalid/#fragment", "https://example.invalid/a/../b", "https://example.invalid/%2fsecret"} {
		t.Run(raw, func(t *testing.T) {
			e := Endpoint{URL: raw, AllowedIPs: []string{"93.184.216.34"}}
			if _, err := endpointURL(e); err == nil {
				t.Fatal("unsafe endpoint accepted")
			}
		})
	}
	t.Run("private_network_requires_explicit_permission", func(t *testing.T) {
		if _, err := endpointURL(Endpoint{URL: "https://localhost", AllowedIPs: []string{"127.0.0.1"}}); err == nil {
			t.Fatal("implicit private network access")
		}
	})
	t.Run("checked_dns_is_pinned_and_redirect_is_not_followed", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			if r.URL.Path == "/redirect" {
				http.Redirect(w, r, "http://169.254.169.254/", 302)
				return
			}
			w.WriteHeader(204)
		}))
		defer server.Close()
		e := Endpoint{URL: strings.Replace(server.URL, "127.0.0.1", "monitor.invalid", 1), AllowedIPs: []string{"127.0.0.1"}, AllowPrivate: true, AllowHTTP: true}
		dns := &fixedDNS{[]netip.Addr{netip.MustParseAddr("127.0.0.1")}}
		client, _, err := endpointClient(e, time.Second, dns)
		if err != nil {
			t.Fatal(SafeCode(err))
		}
		defer client.CloseIdleConnections()
		res, err := client.Get(e.URL + "/redirect")
		if err != nil {
			t.Fatal("approved IP not dialed")
		}
		res.Body.Close()
		if res.StatusCode != 302 || hits.Load() != 1 {
			t.Fatal("redirect followed")
		}
		client.CloseIdleConnections()
		// Use a fresh transport so denial always checks a new DNS lookup.
		deniedClient, _, err := endpointClient(e, time.Second, &fixedDNS{[]netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("169.254.169.254")}})
		if err != nil {
			t.Fatal(SafeCode(err))
		}
		defer deniedClient.CloseIdleConnections()
		if res, err = deniedClient.Get(e.URL); err == nil {
			res.Body.Close()
			t.Fatal("DNS rebinding accepted")
		}
		if hits.Load() != 1 {
			t.Fatal("denied address contacted")
		}
	})
	t.Run("environment_proxy_is_ignored", func(t *testing.T) {
		t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
		defer server.Close()
		e := Endpoint{URL: strings.Replace(server.URL, "127.0.0.1", "monitor.invalid", 1), AllowedIPs: []string{"127.0.0.1"}, AllowPrivate: true, AllowHTTP: true}
		client, _, _ := endpointClient(e, time.Second, &fixedDNS{[]netip.Addr{netip.MustParseAddr("127.0.0.1")}})
		defer client.CloseIdleConnections()
		res, err := client.Get(e.URL)
		if err != nil {
			t.Fatal("implicit proxy used")
		}
		res.Body.Close()
	})
}

func TestStatusBridgeConfigurationBoundaries(t *testing.T) {
	f := newBridgeFixture(t)
	for _, value := range []string{`{"version":1,"version":1}`, `{"version":1} {}`, `null`, `{"version":1,"unexpected":true}`} {
		file := filepath.Join(t.TempDir(), "config.json")
		_ = os.WriteFile(file, []byte(value), 0600)
		if _, err := LoadConfig(file); err == nil {
			t.Fatal("ambiguous or incomplete configuration accepted")
		}
	}
	c := f.c
	c.Targets = append(c.Targets, c.Targets[0])
	if c.Validate() == nil {
		t.Fatal("same source configured twice")
	}
	c = f.c
	c.MaxSampleAge = 181
	if c.Validate() == nil {
		t.Fatal("unbounded sample age")
	}
	c = f.c
	c.Targets[0].Labels["__name__"] = "arbitrary"
	if c.Validate() == nil {
		t.Fatal("metric override accepted")
	}
	delete(c.Targets[0].Labels, "__name__")
	b := openFixture(t, f)
	f.c.Targets[0].Labels["region"] = "caller-mutated"
	if b.config.Targets[0].Labels["region"] != "test" {
		t.Fatal("live query identity aliases caller configuration")
	}
	for _, raw := range []string{"NaN", "+Inf", "1e99999999", "0", "1.0001", "-1", strings.Repeat("1", 1000)} {
		if _, err := millisecondTime(raw); err == nil {
			t.Fatal("invalid timestamp accepted")
		}
	}
}
