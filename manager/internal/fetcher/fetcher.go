package fetcher

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const MaxResponse = 16 << 20

type Result struct {
	Body         []byte
	ETag         string
	LastModified string
	NotModified  bool
}

func Fetch(rawURL, ua, etag, lastModified string, viaProxy bool, proxyPort int) (Result, error) {
	u, e := url.Parse(rawURL)
	if e != nil || u.Scheme != "http" && u.Scheme != "https" {
		return Result{}, fmt.Errorf("subscription URL must be HTTP or HTTPS")
	}
	tr := &http.Transport{}
	if viaProxy {
		if proxyPort <= 0 {
			return Result{}, fmt.Errorf("mihomo mixed-port is unavailable for proxy update")
		}
		p, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
		tr.Proxy = http.ProxyURL(p)
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}}
	req, e := http.NewRequest(http.MethodGet, rawURL, nil)
	if e != nil {
		return Result{}, fmt.Errorf("invalid subscription request")
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, e := client.Do(req)
	if e != nil {
		return Result{}, fmt.Errorf("subscription request failed: %w", redactError(e))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return Result{NotModified: true, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}
	r := io.LimitReader(resp.Body, MaxResponse+1)
	b, e := io.ReadAll(r)
	if e != nil {
		return Result{}, e
	}
	if len(b) > MaxResponse {
		return Result{}, fmt.Errorf("subscription exceeds 16 MiB")
	}
	return Result{Body: b, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}, nil
}

func redactError(err error) error {
	if err == nil {
		return nil
	}
	// url.Error may include the full request URL, which can contain a token.
	// Keep the network cause while never returning subscription credentials.
	if e, ok := err.(*url.Error); ok {
		return fmt.Errorf("%s", e.Err)
	}
	return err
}
