package config

import (
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/profile"
	"strings"
	"testing"
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
func TestManagedTUNStackMigratesLegacyMixed(t *testing.T) {
	s := profile.DefaultSettings()
	s.TUN.Stack = "mixed"
	out, err := (Compiler{Settings: s}).Compile([]byte("mode: rule\n"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "stack: gvisor") {
		t.Fatalf("legacy mixed stack was not migrated: %s", out)
	}
	if strings.Contains(string(out), "stack: mixed") {
		t.Fatalf("legacy mixed stack leaked into compiled config: %s", out)
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
