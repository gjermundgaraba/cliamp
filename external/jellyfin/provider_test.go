package jellyfin

import (
	"net/http"
	"testing"

	"cliamp/playlist"
	"cliamp/provider"
)

func TestProviderName(t *testing.T) {
	p := newProvider(NewClient("https://jf.example.com", "tok", "user-1", "", ""))
	if p.Name() != "Jellyfin" {
		t.Fatalf("Name() = %q, want Jellyfin", p.Name())
	}
}

func TestProviderPlaylists(t *testing.T) {
	p := newProvider(NewClient("https://jf.example.com", "tok", "", "", ""))
	useTestClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/Users/Me":
			return jsonResponse(`{"Id":"user-1","Name":"Nomad"}`), nil
		case "/Users/user-1/Views":
			return jsonResponse(`{"Items":[{"Id":"lib-1","Name":"Music","CollectionType":"music"}]}`), nil
		case "/Items":
			return jsonResponse(`{"Items":[{"Id":"album-1","Name":"Kind of Blue","AlbumArtist":"Miles Davis","ProductionYear":1959,"ChildCount":5}]}`), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	})

	lists, err := p.Playlists()
	if err != nil {
		t.Fatalf("Playlists() error: %v", err)
	}
	if len(lists) != 1 {
		t.Fatalf("expected 1 playlist, got %d", len(lists))
	}
	if lists[0].ID != "album-1" || lists[0].TrackCount != 5 {
		t.Fatalf("playlist = %+v", lists[0])
	}
	if lists[0].Name != "Miles Davis — Kind of Blue (1959)" {
		t.Fatalf("playlist name = %q", lists[0].Name)
	}
}

func TestProviderTracks(t *testing.T) {
	p := newProvider(NewClient("https://jf.example.com", "tok", "user-1", "", ""))
	useTestClient(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/Items" {
			t.Fatalf("unexpected path %s", req.URL.Path)
		}
		return jsonResponse(`{
			"Items": [
				{
					"Id":"track-1",
					"Name":"So What",
					"Album":"Kind of Blue",
					"Artists":["Miles Davis"],
					"ProductionYear":1959,
					"IndexNumber":1,
					"RunTimeTicks":5650000000
				}
			]
		}`), nil
	})

	tracks, err := p.Tracks("album-1")
	if err != nil {
		t.Fatalf("Tracks() error: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	tr := tracks[0]
	if tr.Title != "So What" || tr.Artist != "Miles Davis" || tr.Album != "Kind of Blue" || tr.TrackNumber != 1 || !tr.Stream {
		t.Fatalf("track = %+v", tr)
	}
	if tr.Artwork.CacheKey != "jellyfin:track-1" {
		t.Fatalf("track artwork cache key = %q, want jellyfin:track-1", tr.Artwork.CacheKey)
	}
	if tr.Owner != (playlist.TrackOwner{Provider: provider.KeyJellyfin, ID: "track-1"}) {
		t.Fatalf("track owner = %+v, want jellyfin/track-1", tr.Owner)
	}
}

func TestProviderCanReportPlayback(t *testing.T) {
	p := newProvider(NewClient("https://jf.example.com", "tok", "user-1", "", ""))
	if !p.CanReportPlayback(trackWithOwner(provider.KeyJellyfin, "track-1")) {
		t.Fatal("CanReportPlayback() = false, want true")
	}
	if p.CanReportPlayback(trackWithOwner(provider.KeyNavidrome, "nav-1")) {
		t.Fatal("CanReportPlayback() = true for non-Jellyfin track")
	}
}

func TestProviderResolveArtwork(t *testing.T) {
	p := newProvider(NewClient("https://jf.example.com", "tok", "user-1", "", ""))

	ref, err := p.ResolveArtwork(nil, trackWithOwner(provider.KeyJellyfin, "track-1"))
	if err != nil {
		t.Fatalf("ResolveArtwork() error: %v", err)
	}
	if ref.CacheKey != "jellyfin:track-1" {
		t.Fatalf("artwork cache key = %q, want jellyfin:track-1", ref.CacheKey)
	}
	if ref.URL != "https://jf.example.com/Items/track-1/Images/Primary?api_key=tok" {
		t.Fatalf("artwork url = %q", ref.URL)
	}
}

func TestProviderResolveArtworkWithoutOwner(t *testing.T) {
	p := newProvider(NewClient("https://jf.example.com", "tok", "user-1", "", ""))

	ref, err := p.ResolveArtwork(nil, playlist.Track{
		Path: "https://jf.example.com/Items/track-1/Download?api_key=tok",
	})
	if err != nil {
		t.Fatalf("ResolveArtwork() error: %v", err)
	}
	if !ref.IsNone() {
		t.Fatalf("ResolveArtwork() ref = %+v, want none", ref)
	}
}
func trackWithOwner(providerKey, id string) playlist.Track {
	return playlist.Track{Owner: playlist.TrackOwner{Provider: providerKey, ID: id}}
}
