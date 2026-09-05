package fetcher

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestETag304(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == "v1" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", "v1")
		_, _ = w.Write([]byte("mode: rule\n"))
	}))
	defer srv.Close()
	a, e := Fetch(srv.URL, "", "", "", false, 0)
	if e != nil || a.NotModified || a.ETag != "v1" {
		t.Fatalf("first fetch: %#v %v", a, e)
	}
	b, e := Fetch(srv.URL, "", "v1", "", false, 0)
	if e != nil || !b.NotModified {
		t.Fatalf("304: %#v %v", b, e)
	}
}
