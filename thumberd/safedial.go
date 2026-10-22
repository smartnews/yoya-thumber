package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// Upstream image fetches (url= and io=) must only reach public internet addresses.
// The check runs in the dialer's Control hook, i.e. on the resolved IP of every
// connection, so it also covers DNS names pointing at internal addresses, DNS
// rebinding and redirects to internal hosts.

var errProhibitedAddress = errors.New("destination address is prohibited")

const maxRedirects = 5

// Non-public ranges not covered by the net.IP helpers used in isPublicIP.
var blockedNets = mustParseCIDRs(
	"0.0.0.0/8",      // "this" network
	"100.64.0.0/10",  // carrier-grade NAT
	"192.0.0.0/24",   // IETF protocol assignments
	"198.18.0.0/15",  // benchmarking
	"240.0.0.0/4",    // reserved, incl. broadcast
	"64:ff9b::/96",   // NAT64, can map to internal IPv4
	"64:ff9b:1::/48", // local-use NAT64
)

var publicOnlyDialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
	Control:   publicOnlyControl,
}

// publicOnlyTransport mirrors http.DefaultTransport but only dials public addresses.
// Proxy is intentionally not taken from the environment, otherwise the dialer would
// validate the proxy's address instead of the upstream's.
var publicOnlyTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DialContext = publicOnlyDialer.DialContext
	return t
}()

func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

func dialTLSPublicOnly(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
	d := &tls.Dialer{NetDialer: publicOnlyDialer, Config: cfg}
	return d.DialContext(ctx, network, addr)
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("stopped after too many redirects")
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return errors.New("redirect scheme is prohibited: " + req.URL.Scheme)
	}
	return nil
}

func publicOnlyControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !isPublicIP(ip) {
		return fmt.Errorf("%w: %s", errProhibitedAddress, host)
	}
	return nil
}

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		nets = append(nets, n)
	}
	return nets
}
