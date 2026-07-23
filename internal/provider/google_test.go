package provider

import (
	"errors"
	"testing"
)

func newTestProvider(allowed ...string) *GoogleProvider {
	return NewGoogleProvider("client-id", "client-secret", "postmessage", allowed)
}

func TestConfigFor_EmptyRedirectKeepsThePopupDefault(t *testing.T) {
	p := newTestProvider("https://app.example.com")

	cfg, err := p.configFor("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RedirectURL != "postmessage" {
		t.Fatalf("RedirectURL = %q, want postmessage", cfg.RedirectURL)
	}
	if cfg != p.config {
		t.Fatal("the default config should be reused, not cloned")
	}
}

func TestConfigFor_AllowedRedirectIsUsedWithoutMutatingTheDefault(t *testing.T) {
	p := newTestProvider("https://app.example.com")

	cfg, err := p.configFor("https://app.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RedirectURL != "https://app.example.com" {
		t.Fatalf("RedirectURL = %q", cfg.RedirectURL)
	}
	if p.config.RedirectURL != "postmessage" {
		t.Fatalf("the shared config was mutated: RedirectURL = %q", p.config.RedirectURL)
	}
	if cfg.ClientID != "client-id" || cfg.ClientSecret != "client-secret" {
		t.Fatal("credentials were lost when cloning the config")
	}
	if len(cfg.Scopes) != len(p.config.Scopes) {
		t.Fatal("scopes were lost when cloning the config")
	}
}

func TestConfigFor_TrailingSlashIsNormalisedOnBothSides(t *testing.T) {
	// Registered with a slash, requested without — and the reverse.
	p := newTestProvider("https://app.example.com/")

	for _, requested := range []string{"https://app.example.com", "https://app.example.com/"} {
		cfg, err := p.configFor(requested)
		if err != nil {
			t.Fatalf("configFor(%q): unexpected error: %v", requested, err)
		}
		if cfg.RedirectURL != "https://app.example.com" {
			t.Fatalf("configFor(%q): RedirectURL = %q", requested, cfg.RedirectURL)
		}
	}
}

func TestConfigFor_RejectsAnythingNotRegistered(t *testing.T) {
	p := newTestProvider("https://app.example.com")

	cases := []struct {
		name string
		uri  string
	}{
		{"attacker origin", "https://evil.example.com"},
		{"subdomain of an allowed host", "https://evil.app.example.com"},
		{"allowed host with an added path", "https://app.example.com/steal"},
		{"scheme downgrade", "http://app.example.com"},
		{"open-redirect style suffix", "https://app.example.com.evil.com"},
		{"userinfo trick", "https://app.example.com@evil.com"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := p.configFor(tc.uri)
			if err == nil {
				t.Fatalf("configFor(%q) was accepted with RedirectURL %q", tc.uri, cfg.RedirectURL)
			}
			if !errors.Is(err, ErrRedirectURINotAllowed) {
				t.Fatalf("error = %v, want ErrRedirectURINotAllowed", err)
			}
		})
	}
}

func TestConfigFor_EmptyAllowlistRejectsEveryExplicitRedirect(t *testing.T) {
	p := newTestProvider()

	if _, err := p.configFor("https://app.example.com"); !errors.Is(err, ErrRedirectURINotAllowed) {
		t.Fatalf("error = %v, want ErrRedirectURINotAllowed", err)
	}

	// The popup flow must keep working when no redirects are configured.
	if _, err := p.configFor(""); err != nil {
		t.Fatalf("popup flow broke with an empty allowlist: %v", err)
	}
}
