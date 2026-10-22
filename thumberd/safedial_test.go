package main

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", "::ffff:127.0.0.1",
		"0.0.0.0", "::",
		"10.1.2.3", "172.16.0.1", "192.168.1.1", "fd00:ec2::254",
		"169.254.169.254", "fe80::1",
		"100.64.0.1", "198.18.0.1", "255.255.255.255", "224.0.0.1",
		"64:ff9b::a9fe:a9fe",
	}
	for _, s := range blocked {
		if isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}

	allowed := []string{"52.219.0.1", "8.8.8.8", "2606:4700::1111"}
	for _, s := range allowed {
		if !isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestPublicOnlyTransportRejectsInternalAddresses(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer origin.Close()

	client := http.Client{Transport: publicOnlyTransport, CheckRedirect: checkRedirect}
	_, err := client.Get(origin.URL)
	if !errors.Is(err, errProhibitedAddress) {
		t.Errorf("expected errProhibitedAddress, got %v", err)
	}
}

func TestCheckRedirect(t *testing.T) {
	req := httptest.NewRequest("GET", "ftp://example.com/", nil)
	if checkRedirect(req, nil) == nil {
		t.Error("non-http redirect should be rejected")
	}

	req = httptest.NewRequest("GET", "https://example.com/", nil)
	if err := checkRedirect(req, nil); err != nil {
		t.Errorf("https redirect should be allowed, got %v", err)
	}
	if checkRedirect(req, make([]*http.Request, maxRedirects)) == nil {
		t.Error("redirect chain beyond maxRedirects should be rejected")
	}
}
