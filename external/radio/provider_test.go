package radio

import (
	"testing"

	"cliamp/internal/source"
)

func TestPlaylistsReturnStableSourceIDs(t *testing.T) {
	p := &Provider{
		stations:  []station{{name: "Local", url: "https://local.example.com/stream"}},
		favorites: &Favorites{byURL: make(map[string]struct{})},
		catalog:   []CatalogStation{{Name: "Catalog", URL: "https://catalog.example.com/stream"}},
	}

	lists, err := p.Playlists()
	if err != nil {
		t.Fatalf("Playlists() error = %v", err)
	}
	if len(lists) < 2 {
		t.Fatalf("Playlists() returned %d items, want at least 2", len(lists))
	}

	gotByName := make(map[string]string, len(lists))
	for _, list := range lists {
		gotByName[list.Name] = list.SourceID
	}
	if gotByName["Local"] != sourceIDForURL("https://local.example.com/stream") {
		t.Fatalf("local station SourceID = %q, want URL-based source ID", gotByName["Local"])
	}
	if gotByName["Catalog"] != sourceIDForURL("https://catalog.example.com/stream") {
		t.Fatalf("catalog station SourceID = %q, want URL-based source ID", gotByName["Catalog"])
	}
}

func TestRestoreSourceSupportsURLBackedIDsWithoutSearchState(t *testing.T) {
	p := &Provider{
		favorites: &Favorites{byURL: make(map[string]struct{})},
	}

	tracks, err := p.RestoreSource(source.Ref{
		ProviderKey: "radio",
		Kind:        source.Playlist,
		ID:          sourceIDForURL("https://radio.example.com/stream"),
	})
	if err != nil {
		t.Fatalf("RestoreSource() error = %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("track count = %d, want 1", len(tracks))
	}
	if tracks[0].Path != "https://radio.example.com/stream" {
		t.Fatalf("track path = %q, want saved station URL", tracks[0].Path)
	}
	if !tracks[0].IsLive() {
		t.Fatal("restored radio track should remain live")
	}
}
