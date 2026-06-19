package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRegistryBlock_Source covers the env-var override for a registry location
// (DEX_REGISTRY_<NAME> and the generic DEX_REGISTRY), plus the file/url fallback.
func TestRegistryBlock_Source(t *testing.T) {
	// Make sure no ambient override leaks in.
	t.Setenv("DEX_REGISTRY", "")
	t.Setenv("DEX_REGISTRY_GROVE", "")

	reg := RegistryBlock{Name: "grove", Path: "../dex-registry"}

	// 1. No env: falls back to the block's path as a file: source.
	if got := reg.Source(); got != "file:../dex-registry" {
		t.Fatalf("no env: Source() = %q, want file:../dex-registry", got)
	}

	// 2. Generic DEX_REGISTRY overrides; a bare path becomes a file: source.
	t.Setenv("DEX_REGISTRY", "/srv/registry")
	if got := reg.Source(); got != "file:/srv/registry" {
		t.Fatalf("generic env: Source() = %q, want file:/srv/registry", got)
	}

	// 3. Per-registry DEX_REGISTRY_<NAME> wins over the generic one.
	t.Setenv("DEX_REGISTRY_GROVE", "https://example.com/reg")
	if got := reg.Source(); got != "https://example.com/reg" {
		t.Fatalf("per-registry env: Source() = %q, want the https URL", got)
	}

	// 4. The env key normalizes non-alphanumerics to "_".
	t.Setenv("DEX_REGISTRY_GROVE", "")
	hyphen := RegistryBlock{Name: "my-reg", URL: "https://fallback"}
	t.Setenv("DEX_REGISTRY_MY_REG", "/local/myreg")
	if got := hyphen.Source(); got != "file:/local/myreg" {
		t.Fatalf("hyphen name: Source() = %q, want file:/local/myreg", got)
	}
}

// TestNormalizeRegistrySource_HomeAndScheme checks "~/" expansion and that an
// existing scheme is preserved.
func TestNormalizeRegistrySource_HomeAndScheme(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := normalizeRegistrySource("~/Grove/dex-registry"); got != "file:"+filepath.Join(home, "Grove/dex-registry") {
		t.Fatalf("~/ expansion: got %q", got)
	}
	for _, in := range []string{"file:/a", "https://x/y", "git+ssh://h/r", "s3://b/k"} {
		if got := normalizeRegistrySource(in); got != in {
			t.Fatalf("scheme %q changed to %q", in, got)
		}
	}
}
