package fetcher

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const MaxResponse = 16 << 20

// Tests may opt into loopback httptest servers; production keeps the default
// false and rejects private/local destination literals.
var allowPrivateHosts = os.Getenv("OMARCHY_MIHOMO_ALLOW_PRIVATE_HOSTS") == "1"

func validateHTTPURL(rawURL string, redirect bool) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || strings.TrimSpace(u.Hostname()) == "" {
		if redirect {
			return nil, fmt.Errorf("subscription redirect must be HTTP or HTTPS")
		}
		return nil, fmt.Errorf("subscription URL must be HTTP or HTTPS")
	}
	return u, nil
}

func validateRedirectURL(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || strings.TrimSpace(u.Hostname()) == "" {
		return fmt.Errorf("subscription redirect must be HTTP or HTTPS")
	}
	if err := rejectPrivateHost(u.Hostname()); err != nil && !allowPrivateHosts {
		return err
	}
	return nil
}

func isPrivateAddress(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func rejectPrivateHost(host string) error {
	name := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(host, ".")))
	if name == "localhost" || strings.HasSuffix(name, ".localhost") || strings.HasSuffix(name, ".local") || isPrivateAddress(net.ParseIP(name)) {
		return fmt.Errorf("subscription URL host is not allowed")
	}
	return nil
}

type Result struct {
	Body                []byte
	ETag                string
	LastModified        string
	NotModified         bool
	SubscriptionInfo    SubscriptionInfo
	HasSubscriptionInfo bool
}

type SubscriptionInfo struct {
	Upload   int64
	Download int64
	Total    int64
	Expire   int64
}

func parseSubscriptionInfo(raw string) (SubscriptionInfo, bool) {
	var out SubscriptionInfo
	found := false
	for _, item := range strings.Split(raw, ";") {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		value, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || value < 0 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(parts[0])) {
		case "upload":
			out.Upload, found = value, true
		case "download":
			out.Download, found = value, true
		case "total":
			out.Total, found = value, true
		case "expire":
			out.Expire, found = value, true
		}
	}
	return out, found
}

func Fetch(rawURL, ua, etag, lastModified string, viaProxy bool, proxyPort int) (Result, error) {
	u, err := validateHTTPURL(rawURL, false)
	if err != nil {
		return Result{}, err
	}
	if err = rejectPrivateHost(u.Hostname()); err != nil && !allowPrivateHosts {
		return Result{}, err
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
		if err := validateRedirectURL(req.URL); err != nil {
			return err
		}
		return nil
	}}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
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
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("subscription request failed: %w", redactError(err))
	}
	defer resp.Body.Close()
	usage, hasUsage := parseSubscriptionInfo(resp.Header.Get("subscription-userinfo"))
	if resp.StatusCode == http.StatusNotModified {
		return Result{NotModified: true, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified"), SubscriptionInfo: usage, HasSubscriptionInfo: hasUsage}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}
	r := io.LimitReader(resp.Body, MaxResponse+1)
	body, err := io.ReadAll(r)
	if err != nil {
		return Result{}, err
	}
	if len(body) > MaxResponse {
		return Result{}, fmt.Errorf("subscription exceeds 16 MiB")
	}
	return Result{Body: body, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified"), SubscriptionInfo: usage, HasSubscriptionInfo: hasUsage}, nil
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
