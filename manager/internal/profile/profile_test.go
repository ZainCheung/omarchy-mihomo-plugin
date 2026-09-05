package profile

import (
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/store"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDefaultSettingsUseGVisorTUNStack(t *testing.T) {
	if got := DefaultSettings().TUN.Stack; got != DefaultTUNStack {
		t.Fatalf("default TUN stack = %q, want %q", got, DefaultTUNStack)
	}
}

func TestNormalizeTUNStackMigratesMixedAndEmpty(t *testing.T) {
	for _, input := range []string{"", "mixed", " MIXED "} {
		if got := NormalizeTUNStack(input); got != DefaultTUNStack {
			t.Errorf("NormalizeTUNStack(%q) = %q, want %q", input, got, DefaultTUNStack)
		}
	}
	if got := NormalizeTUNStack("system"); got != "system" {
		t.Fatalf("NormalizeTUNStack(system) = %q", got)
	}
}

func TestLoadSettingsMigratesMixedStack(t *testing.T) {
	s := &store.Store{Home: t.TempDir()}
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	legacy := DefaultSettings()
	legacy.TUN.Stack = "mixed"
	if err := s.WriteJSON(s.SettingsPath(), legacy); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	if got.TUN.Stack != DefaultTUNStack {
		t.Fatalf("loaded TUN stack = %q, want %q", got.TUN.Stack, DefaultTUNStack)
	}
	persisted, err := os.ReadFile(s.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(persisted), `"stack": "gvisor"`) {
		t.Fatalf("settings migration was not persisted: %s", persisted)
	}
}

func TestDueAndUUIDProfile(t *testing.T) {
	id, e := store.RandomID()
	if e != nil || !store.ValidID(id) {
		t.Fatalf("bad UUID: %q %v", id, e)
	}
	m := Meta{ID: id, Type: "remote", UpdateIntervalSec: 60, LastSuccessAt: time.Now().Add(-time.Minute * 2).UTC().Format(time.RFC3339)}
	if !Due(m, time.Now()) {
		t.Fatal("profile should be due")
	}
}
