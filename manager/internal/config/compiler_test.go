package config

import (
	"strings"
	"testing"

	"github.com/ZainCheung/omarchy-mihomo-plugin/manager/internal/policy"
	"github.com/ZainCheung/omarchy-mihomo-plugin/manager/internal/profile"
	"github.com/ZainCheung/omarchy-mihomo-plugin/manager/internal/rules"
)

func TestManagedDNSAndTUNAndProtectedFields(t *testing.T) {
	c := Compiler{Settings: profile.DefaultSettings(), Protected: map[string]any{"external-controller": "127.0.0.1:9090", "secret": "keep"}}
	out, e := c.Compile(CompileInput{Source: []byte("mode: rule\nexternal-controller: evil:1\ndns:\n  enable: false\n"), ProfileOverride: []byte("{}\n")})
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
	out, err := (Compiler{Settings: s}).Compile(CompileInput{Source: source})
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
	out, err := (Compiler{Settings: s}).Compile(CompileInput{Source: []byte("mode: rule\n")})
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
	out, e := (Compiler{Settings: s}).Compile(CompileInput{Source: []byte("dns:\n  enable: false\ntun:\n  enable: false\n")})
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

func TestCompileLayersCustomRulesWithoutReplacingSourceRules(t *testing.T) {
	source := []byte(`mode: rule
proxy-groups:
  - name: Select
    type: select
rules:
  - GEOIP,CN,DIRECT
  - MATCH,Select
`)
	global := []byte("log-level: warning\n")
	profileOverride := []byte("allow-lan: true\n")
	custom := []rules.Rule{
		{Enabled: true, Match: rules.Match{Type: rules.DomainSuffix, Value: "openai.com"}, Policy: rules.Direct},
		{Enabled: true, Match: rules.Match{Type: rules.Domain, Value: "example.com"}, Policy: rules.Proxy},
	}
	out, err := (Compiler{}).Compile(CompileInput{
		Source: source, GlobalOverride: global, ProfileOverride: profileOverride,
		CustomRules: custom, Bindings: policy.Bindings{policy.Proxy: "Select"},
		Settings: profile.Settings{DNSManagement: "inherit", TUNManagement: "inherit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if compiled["log-level"] != "warning" || compiled["allow-lan"] != true {
		t.Fatalf("override layers were not applied: %#v", compiled)
	}
	gotRules, ok := compiled["rules"].([]any)
	if !ok {
		t.Fatalf("compiled rules are not an array: %#v", compiled["rules"])
	}
	want := []any{
		"DOMAIN-SUFFIX,openai.com,DIRECT",
		"DOMAIN,example.com,Select",
		"GEOIP,CN,DIRECT",
		"MATCH,Select",
	}
	if len(gotRules) != len(want) {
		t.Fatalf("compiled rules length = %d, want %d: %#v", len(gotRules), len(want), gotRules)
	}
	for i := range want {
		if gotRules[i] != want[i] {
			t.Fatalf("compiled rule %d = %#v, want %#v", i, gotRules[i], want[i])
		}
	}
}
