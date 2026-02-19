package tests

import (
	"io"
	"net/http"
	"testing"
	"time"
	"crypto/x509"

	"example.com/pisag_go/pisag"
	"example.com/pisag_go/ports"
)

func TestRequestGuard_AllowOraclePath(t *testing.T) {
	policy := ports.Policy{
		AllowedHosts: []ports.AllowedHost{
			{
				Host:         "oracle.singularity.local",
				Port:         443,
				PathPrefixes: []string{"/v1/catalog/"},
			},
		},
		// For docker-compose dev only: allow your compose network CIDR here.
		// Replace with your actual network range if needed.
		AllowCIDRs:   []string{"172.18.0.0/16"},
		MaxRedirects: 3,
		Timeout:      10 * time.Second,
	}

	u, err := pisag.RequestGuard("https://oracle.singularity.local/v1/catalog/pricing_v1.json", policy)
	if err != nil {
		t.Fatalf("expected allow, got err=%v", err)
	}
	if u.Hostname() != "oracle.singularity.local" {
		t.Fatalf("unexpected hostname: %s", u.Hostname())
	}
}

func TestRequestGuard_DenyHTTP(t *testing.T) {
	policy := ports.Policy{
		AllowedHosts: []ports.AllowedHost{{Host: "oracle.singularity.local", Port: 443, PathPrefixes: []string{"/v1/"}}},
	}
	_, err := pisag.RequestGuard("http://oracle.singularity.local/v1/catalog/pricing_v1.json", policy)
	if err == nil {
		t.Fatal("expected deny for non-https")
	}
}

func TestRequestGuard_DenyHost(t *testing.T) {
	policy := ports.Policy{
		AllowedHosts: []ports.AllowedHost{{Host: "oracle.singularity.local", Port: 443, PathPrefixes: []string{"/v1/"}}},
	}
	_, err := pisag.RequestGuard("https://evil.example.com/v1/catalog/pricing_v1.json", policy)
	if err == nil {
		t.Fatal("expected deny for host not allowed")
	}
}

func TestRequestGuard_DenyPath(t *testing.T) {
	policy := ports.Policy{
		AllowedHosts: []ports.AllowedHost{{Host: "oracle.singularity.local", Port: 443, PathPrefixes: []string{"/v1/catalog/"}}},
	}
	_, err := pisag.RequestGuard("https://oracle.singularity.local/admin/secret", policy)
	if err == nil {
		t.Fatal("expected deny for path not allowed")
	}
}

func TestClient_RedirectHostChangeDenied(t *testing.T) {
	// serverA redirects to serverB (host change) -> must be denied by PISAG client CheckRedirect.
	serverB, hostB := newTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("B"))
	}))
	defer serverB.Close()

	serverA, hostA := newTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+hostB+"/v1/catalog/pricing_v1.json", http.StatusFound)
	}))
	defer serverA.Close()

	policy := ports.Policy{
		AllowedHosts: []ports.AllowedHost{
			// allow only serverA host:port and prefix
			{Host: hostname(hostA), Port: port(hostA), PathPrefixes: []string{"/"}},
		},
		AllowCIDRs:   []string{"127.0.0.1/32"}, // test server is loopback; dev-only override
		MaxRedirects: 3,
		Timeout:      5 * time.Second,
	}

	client, err := pisag.NewClient(policy)
	if err != nil {
		t.Fatalf("NewClient err=%v", err)
	}

	// Must pass RequestGuard before doing the request in real usage.
	_, gerr := pisag.RequestGuard("https://"+hostA+"/start", policy)
	if gerr != nil {
		t.Fatalf("guard err=%v", gerr)
	}

	resp, rerr := client.Get("https://" + hostA + "/start")
	if rerr == nil {
		_ = resp.Body.Close()
		t.Fatal("expected redirect deny, got success")
	}
}

func TestClient_BasicGETAgainstTLSServer(t *testing.T) {
	server, host := newTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Build RootCAs from server cert (no InsecureSkipVerify).
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())

	policy := ports.Policy{
		AllowedHosts: []ports.AllowedHost{
			{Host: hostname(host), Port: port(host), PathPrefixes: []string{"/"}},
		},
		AllowCIDRs:   []string{"127.0.0.1/32"},
		MaxRedirects: 3,
		Timeout:      5 * time.Second,
		TLSRootCAs:   pool,
	}

	client, err := pisag.NewClient(policy)
	if err != nil {
		t.Fatalf("NewClient err=%v", err)
	}

	_, gerr := pisag.RequestGuard("https://"+host+"/", policy)
	if gerr != nil {
		t.Fatalf("guard err=%v", gerr)
	}

	resp, err := client.Get("https://" + host + "/")
	if err != nil {
		t.Fatalf("GET err=%v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "OK" {
		t.Fatalf("unexpected body: %s", string(b))
	}
}

// helpers
func hostname(hostPort string) string {
	h, _, _ := splitHostPort(hostPort)
	return h
}
func port(hostPort string) int {
	_, p, _ := splitHostPort(hostPort)
	return p
}