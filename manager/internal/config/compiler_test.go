package config

import (
	"strings"
	"testing"

	"github.com/ZainCheung/omarchy-mihomo-plugin/manager/internal/profile"
)

func TestManagedDNSAndTUNAndProtectedFields(t *testing.T) {
	c := Compiler{Settings: profile.DefaultSettings(), Protected: map[string]any{"external-controller": "127.0.0.1:9090", "secret": "keep"}}
	out, e := c.Compile([]byte("mode: rule\nexternal-controller: evil:1\ndns:\n  enable: false\n"), nil, []byte("{}\n"))
	if e != nil {
		t.Fatal(e)
	}
	s := string(out)
	for _, want := range []string{"dns:", "tun:", "stack: gvisor", "external-controller: 127.0.0.1:9090", "secret: keep"} {
		if !strings.Contains(s, want) {
			t.Fatalf("compiled config lacks %q: %s", want, s)
		}
	}
}
func TestManagedDNSAndTUNPreserveUnknownFields(t *testing.T) {
	s := profile.DefaultSettings()
	s.DNS.FakeIPFilter = []string{"*.lan"}
	s.TUN.DNSHijack = []string{"any:53"}
	source := []byte(`mode: rule
dns:
  respect-rules: true
  nameserver-policy:
    "geosite:cn":
      - 223.5.5.5
  fallback:
    - https://1.1.1.1/dns-query
  fake-ip-filter-mode: blacklist
tun:
  mtu: 1500
  auto-redirect: true
  route:
    strict-route: true
    include-interface:
      - eth0
`)
	out, err := (Compiler{Settings: s}).Compile(source, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	dns, ok := compiled["dns"].(map[string]any)
	if !ok {
		t.Fatalf("dns is not a map: %#v", compiled["dns"])
	}
	if dns["respect-rules"] != true || dns["fake-ip-filter-mode"] != "blacklist" {
		t.Fatalf("managed DNS removed unknown fields: %#v", dns)
	}
	if _, ok := dns["nameserver-policy"].(map[string]any); !ok {
		t.Fatalf("managed DNS removed nameserver-policy: %#v", dns)
	}
	if _, ok := dns["fallback"].([]any); !ok {
		t.Fatalf("managed DNS removed fallback: %#v", dns)
	}
	if dns["enhanced-mode"] != s.DNS.EnhancedMode {
		t.Fatalf("managed DNS field was not applied: %#v", dns)
	}

	tun, ok := compiled["tun"].(map[string]any)
	if !ok {
		t.Fatalf("tun is not a map: %#v", compiled["tun"])
	}
	if tun["mtu"] != 1500 || tun["auto-redirect"] != true {
		t.Fatalf("managed TUN removed unknown fields: %#v", tun)
	}
	route, ok := tun["route"].(map[string]any)
	if !ok || route["strict-route"] != true {
		t.Fatalf("managed TUN removed route fields: %#v", tun)
	}
	if tun["stack"] != profile.DefaultTUNStack {
		t.Fatalf("managed TUN field was not applied: %#v", tun)
	}
}

func TestManagedTUNStackPreservesExplicitMixed(t *testing.T) {
	s := profile.DefaultSettings()
	s.TUN.Stack = "mixed"
	out, err := (Compiler{Settings: s}).Compile([]byte("mode: rule\n"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "stack: mixed") {
		t.Fatalf("explicit mixed stack was not preserved: %s", out)
	}
}

func TestInheritLeavesNetworkConfig(t *testing.T) {
	s := profile.DefaultSettings()
	s.DNSManagement = "inherit"
	s.TUNManagement = "inherit"
	out, e := (Compiler{Settings: s}).Compile([]byte("dns:\n  enable: false\ntun:\n  enable: false\n"), nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	text := string(out)
	if !strings.Contains(text, "enable: false") {
		t.Fatal(text)
	}
	if strings.Contains(text, "198.18.0.1/16") {
		t.Fatal("managed DNS leaked into inherit mode")
	}
}
