package model

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"cliamp/internal/artwork"
	"cliamp/playlist"
	"cliamp/provider"
)

func TestArtworkSessionDropsStaleResults(t *testing.T) {
	var session artworkSession

	gen := session.Activate(0)
	if !session.Apply(artworkResolvedMsg{index: 0, gen: gen, path: "/tmp/cover.png"}) {
		t.Fatal("expected current artwork result to apply")
	}
	if session.path != "/tmp/cover.png" {
		t.Fatalf("session path = %q, want /tmp/cover.png", session.path)
	}

	nextGen := session.Activate(1)
	if session.Apply(artworkResolvedMsg{index: 0, gen: gen, path: "/tmp/stale.png"}) {
		t.Fatal("stale artwork result unexpectedly applied")
	}
	if session.path != "" {
		t.Fatalf("session path after stale result = %q, want empty", session.path)
	}
	if !session.Apply(artworkResolvedMsg{index: 1, gen: nextGen, path: "/tmp/fresh.png"}) {
		t.Fatal("expected fresh artwork result to apply")
	}
}

func TestReplacePlaylistInvalidatesStaleArtworkResults(t *testing.T) {
	m := Model{playlist: playlist.New()}
	m.replacePlaylist([]playlist.Track{{Title: "old"}})

	gen := m.artwork.session.Activate(0)

	m.replacePlaylist([]playlist.Track{{Title: "new"}})

	nextModel, _ := m.Update(artworkResolvedMsg{
		index: 0,
		gen:   gen,
		path:  "/tmp/stale.jpg",
	})

	next := nextModel.(Model)
	if next.artwork.session.path != "" {
		t.Fatalf("session path = %q, want empty", next.artwork.session.path)
	}
}

func TestMoveTrackInvalidatesStaleArtworkResults(t *testing.T) {
	m := Model{playlist: playlist.New()}
	m.replacePlaylist([]playlist.Track{{Title: "first"}, {Title: "second"}})

	gen := m.artwork.session.Activate(0)

	moved, cmd := m.moveTrack(0, 1)
	if !moved {
		t.Fatal("moveTrack returned false, want true")
	}
	if cmd != nil {
		t.Fatalf("moveTrack cmd = %v, want nil without artwork materializer", cmd)
	}

	nextModel, _ := m.Update(artworkResolvedMsg{
		index: 0,
		gen:   gen,
		path:  "/tmp/stale.jpg",
	})

	next := nextModel.(Model)
	if next.artwork.session.path != "" {
		t.Fatalf("session path = %q, want empty after move", next.artwork.session.path)
	}
}

func TestMoveTrackPreservesCurrentArtwork(t *testing.T) {
	tests := []struct {
		name           string
		tracks         []playlist.Track
		from           int
		to             int
		wantSessionIdx int
	}{
		{
			name:           "current track moves",
			tracks:         []playlist.Track{{Title: "first"}, {Title: "second"}},
			from:           0,
			to:             1,
			wantSessionIdx: 1,
		},
		{
			name:           "other tracks move",
			tracks:         []playlist.Track{{Title: "current"}, {Title: "second"}, {Title: "third"}},
			from:           1,
			to:             2,
			wantSessionIdx: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &fakeNotifier{}
			m := Model{
				player:   &fakeEngine{},
				playlist: playlist.New(),
				notifier: notifier,
			}
			m.replacePlaylist(tt.tracks)

			gen := m.artwork.session.Activate(0)
			if !m.artwork.session.Apply(artworkResolvedMsg{index: 0, gen: gen, path: "/tmp/cover.jpg"}) {
				t.Fatal("expected current artwork result to apply")
			}

			moved, cmd := m.moveTrack(tt.from, tt.to)
			if !moved {
				t.Fatal("moveTrack returned false, want true")
			}
			if cmd != nil {
				t.Fatalf("moveTrack cmd = %v, want nil when artwork is already resolved", cmd)
			}
			if m.artwork.session.index != tt.wantSessionIdx {
				t.Fatalf("session index = %d, want %d", m.artwork.session.index, tt.wantSessionIdx)
			}
			if m.artwork.session.path != "/tmp/cover.jpg" {
				t.Fatalf("session path = %q, want /tmp/cover.jpg", m.artwork.session.path)
			}

			m.notifyAll()

			if len(notifier.updates) != 1 {
				t.Fatalf("notifier update count = %d, want 1", len(notifier.updates))
			}
			if got := notifier.updates[0].Track.ArtworkPath; got != "/tmp/cover.jpg" {
				t.Fatalf("notifier artwork path = %q, want /tmp/cover.jpg", got)
			}
		})
	}
}

func TestMoveTrackRefreshesCurrentArtworkWhenTrackMovesMidResolve(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := Model{playlist: playlist.New()}
	m.replacePlaylist([]playlist.Track{
		{
			Title:   "first",
			Artwork: playlist.RemoteArtwork("first", "https://example.com/first.jpg"),
		},
		{Title: "second"},
	})
	m.artwork.materializer = artwork.NewMaterializer()

	gen := m.artwork.session.Activate(0)

	moved, cmd := m.moveTrack(0, 1)
	if !moved {
		t.Fatal("moveTrack returned false, want true")
	}
	if cmd == nil {
		t.Fatal("moveTrack cmd = nil, want artwork refresh command")
	}
	if m.artwork.session.index != 1 {
		t.Fatalf("session index = %d, want 1", m.artwork.session.index)
	}
	if m.artwork.session.gen == gen {
		t.Fatalf("session gen = %d, want new generation after move", m.artwork.session.gen)
	}
	if m.artwork.session.path != "" {
		t.Fatalf("session path = %q, want empty while refresh is pending", m.artwork.session.path)
	}
}

type artworkResolverStub struct {
	ref   playlist.ArtworkRef
	calls int
}

func (s *artworkResolverStub) Name() string { return "stub" }

func (s *artworkResolverStub) Playlists() ([]playlist.PlaylistInfo, error) { return nil, nil }

func (s *artworkResolverStub) Tracks(string) ([]playlist.Track, error) { return nil, nil }

func (s *artworkResolverStub) ResolveArtwork(_ context.Context, _ playlist.Track) (playlist.ArtworkRef, error) {
	s.calls++
	return s.ref, nil
}

func TestRefreshCurrentArtworkUsesProviderResolver(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(tinyPNGBytes(t))
	}))
	defer server.Close()

	resolver := &artworkResolverStub{
		ref: playlist.RemoteArtwork("resolver:test", server.URL+"/cover.png"),
	}

	m := Model{
		playlist: playlist.New(),
		providers: providerState{
			active:  resolver,
			entries: []provider.Entry{{Key: provider.KeySpotify, Provider: resolver}},
		},
	}
	m.replacePlaylist([]playlist.Track{{
		Path:  "spotify:track:abc123",
		Title: "Song",
		Owner: playlist.TrackOwner{Provider: provider.KeySpotify, ID: "abc123"},
	}})
	m.artwork.materializer = artwork.NewMaterializer()

	cmd := m.refreshCurrentArtwork()
	if cmd == nil {
		t.Fatal("refreshCurrentArtwork cmd = nil, want resolver-backed command")
	}

	msg := cmd()
	artMsg, ok := msg.(artworkResolvedMsg)
	if !ok {
		t.Fatalf("cmd() msg = %T, want artworkResolvedMsg", msg)
	}
	m.applyArtworkResolved(artMsg)

	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
	if m.artwork.session.path == "" {
		t.Fatal("artwork session path is empty after resolver materialization")
	}

	track, idx := m.playlist.Current()
	if idx != 0 {
		t.Fatalf("current index = %d, want 0", idx)
	}
	if track.Artwork.URL != server.URL+"/cover.png" {
		t.Fatalf("track artwork url = %q, want %q", track.Artwork.URL, server.URL+"/cover.png")
	}
	if track.Artwork.CacheKey != "resolver:test" {
		t.Fatalf("track artwork cache key = %q, want %q", track.Artwork.CacheKey, "resolver:test")
	}
}

func TestRefreshCurrentArtworkRetriesResolverAfterMaterializeError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "nope", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(tinyPNGBytes(t))
	}))
	defer server.Close()

	resolver := &artworkResolverStub{
		ref: playlist.RemoteArtwork("resolver:retry", server.URL+"/cover.png"),
	}

	m := Model{
		playlist: playlist.New(),
		providers: providerState{
			active:  resolver,
			entries: []provider.Entry{{Key: provider.KeySpotify, Provider: resolver}},
		},
	}
	m.replacePlaylist([]playlist.Track{{
		Path:  "spotify:track:retry",
		Title: "Song",
		Owner: playlist.TrackOwner{Provider: provider.KeySpotify, ID: "retry"},
	}})
	m.artwork.materializer = artwork.NewMaterializer()

	firstCmd := m.refreshCurrentArtwork()
	if firstCmd == nil {
		t.Fatal("first refreshCurrentArtwork cmd = nil, want resolver-backed command")
	}
	firstMsg, ok := firstCmd().(artworkResolvedMsg)
	if !ok {
		t.Fatalf("first cmd() msg = %T, want artworkResolvedMsg", firstCmd())
	}
	if firstMsg.err == nil {
		t.Fatal("first artwork resolve should fail to materialize")
	}
	m.applyArtworkResolved(firstMsg)

	track, idx := m.playlist.Current()
	if idx != 0 {
		t.Fatalf("current index = %d, want 0", idx)
	}
	if !track.Artwork.IsNone() {
		t.Fatalf("track artwork after failed materialize = %+v, want none", track.Artwork)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls after first refresh = %d, want 1", resolver.calls)
	}

	secondCmd := m.refreshCurrentArtwork()
	if secondCmd == nil {
		t.Fatal("second refreshCurrentArtwork cmd = nil, want retry command")
	}
	secondMsg, ok := secondCmd().(artworkResolvedMsg)
	if !ok {
		t.Fatalf("second cmd() msg = %T, want artworkResolvedMsg", secondCmd())
	}
	if secondMsg.err != nil {
		t.Fatalf("second artwork resolve error = %v, want nil", secondMsg.err)
	}
	m.applyArtworkResolved(secondMsg)

	track, idx = m.playlist.Current()
	if idx != 0 {
		t.Fatalf("current index = %d, want 0", idx)
	}
	if resolver.calls != 2 {
		t.Fatalf("resolver calls after retry = %d, want 2", resolver.calls)
	}
	if track.Artwork.URL != server.URL+"/cover.png" {
		t.Fatalf("track artwork url = %q, want %q", track.Artwork.URL, server.URL+"/cover.png")
	}
	if track.Artwork.CacheKey != "resolver:retry" {
		t.Fatalf("track artwork cache key = %q, want %q", track.Artwork.CacheKey, "resolver:retry")
	}
	if m.artwork.session.path == "" {
		t.Fatal("artwork session path is empty after successful retry")
	}
}

func tinyPNGBytes(t *testing.T) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error: %v", err)
	}
	return buf.Bytes()
}
