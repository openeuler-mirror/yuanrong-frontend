package sandboxrouter

import (
	"testing"

	"frontend/pkg/sandboxrouter/config"
)

func TestEffectiveConfigDefaultsNilToEnabled(t *testing.T) {
	cfg := effectiveConfig(nil)
	if cfg == nil {
		t.Fatal("effectiveConfig(nil) returned nil")
	}
	if !cfg.Enabled {
		t.Fatal("nil sandboxrouter config should default to enabled")
	}
}

func TestEffectiveConfigRespectsExplicitDisabled(t *testing.T) {
	input := &config.SandboxRouterConfig{Enabled: false}
	cfg := effectiveConfig(input)
	if cfg != input {
		t.Fatal("effectiveConfig should preserve explicit config pointer")
	}
	if cfg.Enabled {
		t.Fatal("explicit sandboxrouter enabled=false should remain disabled")
	}
}
