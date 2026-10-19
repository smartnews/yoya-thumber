package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsPublicAddress(t *testing.T) {
	cases := map[string]bool{
		"52.219.0.10":       true,
		"2600:9000:2000::1": true,
		"127.0.0.1":         false,
		"0.0.0.0":           false,
		"10.9.0.68":         false,
		"172.16.0.1":        false,
		"192.168.1.1":       false,
		"169.254.169.254":   false,
		"169.254.170.2":     false,
		"100.64.0.1":        false,
		"::1":               false,
		"::":                false,
		"fdaa:9252:9893::1": false,
		"fe80::1":           false,
		"::ffff:127.0.0.1":  false,
		"64:ff9b::a09:44":   false,
		"64:ff9b::34db:a":   true,
		"ff02::1":           false,
		"224.0.0.1":         false,
	}
	for addr, want := range cases {
		if got := isPublicAddress(net.ParseIP(addr)); got != want {
			t.Errorf("isPublicAddress(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestUpstreamTransportRefusesNonPublicAddress(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached a loopback origin")
	}))
	defer origin.Close()

	client := http.Client{Transport: upstreamTransport}
	_, err := client.Get(origin.URL)
	if err == nil || !strings.Contains(err.Error(), "is not public") {
		t.Fatalf("expected the dial to be refused, got %v", err)
	}
}

func TestUpstreamTransportRefusesRedirectToNonPublicAddress(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect reached a loopback origin")
	}))
	defer internal.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	defer redirector.Close()

	// Let the first dial (to the redirector) through and apply the real check to the next one.
	dials := 0
	upstreamAddressAllowed = func(ip net.IP) bool { dials++; return dials == 1 || isPublicAddress(ip) }
	defer func() { upstreamAddressAllowed = isPublicAddress }()

	client := http.Client{Transport: upstreamTransport}
	_, err := client.Get(redirector.URL)
	if err == nil || !strings.Contains(err.Error(), "is not public") {
		t.Fatalf("expected the redirect target to be refused, got %v", err)
	}
}

func TestNegativeDimensionsAreRejected(t *testing.T) {
	ts := httptest.NewServer(&Handler{sem: make(chan int, 1)})
	defer ts.Close()

	for _, q := range []string{"w=-1", "h=-1"} {
		res, err := http.Get(ts.URL + "/?" + q + "&url=example.com/a.jpg")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", q, res.StatusCode)
		}
	}
}
