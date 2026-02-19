package tests

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
)

// newTLSServer returns an HTTPS server and its host:port.
func newTLSServer(handler http.Handler) (*httptest.Server, string) {
	s := httptest.NewTLSServer(handler)
	hostPort := s.Listener.Addr().String()
	return s, hostPort
}

// tlsClientSkipVerify is only for verifying test servers behavior.
// PISAG itself must never use InsecureSkipVerify.
func tlsClientSkipVerify() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				d := &net.Dialer{}
				return d.DialContext(ctx, network, addr)
			},
		},
	}
}