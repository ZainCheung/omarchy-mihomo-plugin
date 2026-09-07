package fetcher

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestETag304(t *testing.T) {
	old := allowPrivateHosts
	allowPrivateHosts = true
	defer func() { allowPrivateHosts = old }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("subscription-userinfo", "upload=10; download=20; total=100; expire=1700000000")
		if r.Header.Get("If-None-Match") == "v1" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", "v1")
		_, _ = w.Write([]byte("mode: rule\n"))
	}))
	defer srv.Close()
	a, e := Fetch(srv.URL, "", "", "", false, 0)
	if e != nil || a.NotModified || a.ETag != "v1" || !a.HasSubscriptionInfo || a.SubscriptionInfo.Download != 20 || a.SubscriptionInfo.Total != 100 {
		t.Fatalf("first fetch: %#v %v", a, e)
	}
	b, e := Fetch(srv.URL, "", "v1", "", false, 0)
	if e != nil || !b.NotModified {
		t.Fatalf("304: %#v %v", b, e)
	}
}

func TestFetchRejectsMalformedOrHostlessURL(t *testing.T) {
	for _, rawURL := range []string{"http://", "https:///path", "http://?token=secret"} {
		if _, err := Fetch(rawURL, "", "", "", false, 0); err == nil {
			t.Fatalf("expected URL validation error for %q", rawURL)
		} else if containsSecret(err.Error()) {
			t.Fatalf("URL credential leaked for %q: %v", rawURL, err)
		}
	}
}

func TestFetchRejectsPrivateHosts(t *testing.T) {
	for _, rawURL := range []string{"http://127.0.0.1/", "http://localhost/", "http://[::1]/", "http://192.168.1.1/"} {
		for _, viaProxy := range []bool{false, true} {
			if _, err := Fetch(rawURL, "", "", "", viaProxy, 7890); err == nil {
				t.Fatalf("expected private host to be rejected for %q (viaProxy=%v)", rawURL, viaProxy)
			}
		}
	}
}

func TestFetchRejectsUserinfo(t *testing.T) {
	if _, err := Fetch("http://user:password@example.com/", "", "", "", false, 0); err == nil {
		t.Fatal("expected userinfo URL to be rejected")
	}
}

func TestRedirectLimit(t *testing.T) {
	old := allowPrivateHosts
	allowPrivateHosts = true
	defer func() { allowPrivateHosts = old }()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := r.URL.Query().Get("n")
		if n == "" {
			n = "0"
		}
		w.Header().Set("Location", srv.URL+"/?n="+n+"x")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	if _, err := Fetch(srv.URL, "", "", "", false, 0); err == nil {
		t.Fatal("expected redirect limit error")
	}
}

func TestRedirectRejectsUnsupportedScheme(t *testing.T) {
	old := allowPrivateHosts
	allowPrivateHosts = true
	defer func() { allowPrivateHosts = old }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "file:///secret/token")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	if _, err := Fetch(srv.URL, "", "", "", false, 0); err == nil || containsSecret(err.Error()) {
		t.Fatalf("unexpected redirect error: %v", err)
	}
}

func TestFetchRejectsPrivateResolvedHost(t *testing.T) {
	oldAllow := allowPrivateHosts
	oldLookup := lookupIP
	allowPrivateHosts = false
	lookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}, nil
	}
	defer func() {
		allowPrivateHosts = oldAllow
		lookupIP = oldLookup
	}()

	if _, err := Fetch("https://subscription.example.test/config.yaml", "", "", "", false, 0); err == nil {
		t.Fatal("expected a hostname resolving to a private address to be rejected")
	}
}

func TestSafeDialRejectsPrivateResolvedHost(t *testing.T) {
	oldAllow := allowPrivateHosts
	oldLookup := lookupIP
	allowPrivateHosts = false
	lookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	defer func() {
		allowPrivateHosts = oldAllow
		lookupIP = oldLookup
	}()

	dial := safeDialContext("")
	if _, err := dial(context.Background(), "tcp", "subscription.example.test:443"); err == nil {
		t.Fatal("expected the custom dialer to reject a private DNS result")
	}
}
