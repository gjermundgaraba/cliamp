package provider

import "testing"

func TestDisplayName(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{KeyRadio, "Radio"},
		{KeyYT, "YouTube (All)"},
		{"custom", "custom"},
	}

	for _, tt := range tests {
		if got := DisplayName(tt.key); got != tt.want {
			t.Errorf("DisplayName(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestNormalizeDefaultProviderKey(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "exact", raw: KeyRadio, want: KeyRadio, ok: true},
		{name: "trimmed and lowercased", raw: "  SpOtIfY ", want: KeySpotify, ok: true},
		{name: "youtube music", raw: "YTMUSIC", want: KeyYTMusic, ok: true},
		{name: "local is not default selectable", raw: KeyLocal, ok: false},
		{name: "unknown", raw: "nope", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeDefaultProviderKey(tt.raw)
			if ok != tt.ok {
				t.Fatalf("NormalizeDefaultProviderKey(%q) ok = %v, want %v", tt.raw, ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("NormalizeDefaultProviderKey(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDefaultProviderUsage(t *testing.T) {
	const want = "radio, navidrome, plex, jellyfin, spotify, yt, youtube, ytmusic"
	if got := DefaultProviderUsage(); got != want {
		t.Fatalf("DefaultProviderUsage() = %q, want %q", got, want)
	}
}
