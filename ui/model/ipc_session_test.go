package model

import (
	"testing"

	"cliamp/internal/session"
	"cliamp/internal/source"
	"cliamp/ipc"
	"cliamp/playlist"
	"cliamp/provider"
)

type localPlaylistProviderStub struct {
	playlists map[string][]playlist.Track
}

func (s localPlaylistProviderStub) Name() string { return "local" }

func (s localPlaylistProviderStub) Playlists() ([]playlist.PlaylistInfo, error) { return nil, nil }

func (s localPlaylistProviderStub) Tracks(name string) ([]playlist.Track, error) {
	return s.playlists[name], nil
}

func TestIPCLoadMsgCancelsPendingRestoreAndSetsLocalPlaylistSource(t *testing.T) {
	pl := playlist.New()
	pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})

	m := Model{
		player:   &fakeEngine{},
		playlist: pl,
		localProvider: localPlaylistProviderStub{
			playlists: map[string][]playlist.Track{
				"mix": {
					{Title: "loaded", Path: "/loaded.mp3"},
				},
			},
		},
		restore: pendingRestore{
			plan: session.RestorePlan{
				Mode: session.RestoreDeferred,
				State: session.State{
					Source: source.Ref{
						ProviderKey: "navidrome",
						Kind:        source.Playlist,
						ID:          "saved-id",
					},
				},
			},
			token: 7,
		},
		source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "saved-id",
		},
	}

	updated, cmd := m.Update(ipc.LoadMsg{Playlist: "mix"})
	if cmd != nil {
		cmd()
	}

	next := updated.(Model)
	if next.restore.pending() {
		t.Fatal("pending restore remained after ipc.LoadMsg")
	}
	if next.restore.token != 8 {
		t.Fatalf("restore token = %d, want 8", next.restore.token)
	}
	wantSource := source.Ref{
		ProviderKey: "local",
		Kind:        source.Playlist,
		ID:          "mix",
	}
	if next.source != wantSource {
		t.Fatalf("source = %+v, want %+v", next.source, wantSource)
	}
	if next.loadedPlaylist != "mix" {
		t.Fatalf("loadedPlaylist = %q, want mix", next.loadedPlaylist)
	}
	track, idx := next.playlist.Current()
	if idx != 0 || track.Path != "/loaded.mp3" {
		t.Fatalf("Current() = (%q, %d), want (/loaded.mp3, 0)", track.Path, idx)
	}
}

func TestIPCQueueMsgClearsSourceTracking(t *testing.T) {
	pl := playlist.New()
	pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})

	m := Model{
		player:   &fakeEngine{},
		playlist: pl,
		restore: pendingRestore{
			plan: session.RestorePlan{
				Mode: session.RestoreDeferred,
				State: session.State{
					Source: source.Ref{
						ProviderKey: "navidrome",
						Kind:        source.Playlist,
						ID:          "saved-id",
					},
				},
			},
			token: 3,
		},
		source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "saved-id",
		},
	}

	updated, cmd := m.Update(ipc.QueueMsg{Path: "/queued.mp3"})
	if cmd != nil {
		t.Fatalf("Update() cmd = %v, want nil", cmd)
	}

	next := updated.(Model)
	if next.restore.pending() {
		t.Fatal("pending restore remained after ipc.QueueMsg")
	}
	if next.restore.token != 4 {
		t.Fatalf("restore token = %d, want 4", next.restore.token)
	}
	if next.source != (source.Ref{}) {
		t.Fatalf("source = %+v, want cleared source", next.source)
	}
	if next.playlist.Len() != 2 {
		t.Fatalf("playlist len = %d, want 2", next.playlist.Len())
	}
	tracks := next.playlist.Tracks()
	if tracks[1].Path != "/queued.mp3" {
		t.Fatalf("queued track path = %q, want /queued.mp3", tracks[1].Path)
	}
}

func TestNavTrackListSourceIgnoresStaleAlbumSelectionForArtistTracks(t *testing.T) {
	m := Model{
		navBrowser: navBrowserState{
			mode:     navBrowseModeByArtist,
			selAlbum: provider.AlbumInfo{ID: "album-1", Name: "Stale Album"},
		},
	}

	if got := m.navTrackListSource(); got != (source.Ref{}) {
		t.Fatalf("navTrackListSource() = %+v, want empty source", got)
	}
}

func TestNavTrackListSourceKeepsAlbumRestoreForAlbumModes(t *testing.T) {
	tests := []struct {
		name string
		mode navBrowseModeType
	}{
		{name: "by album", mode: navBrowseModeByAlbum},
		{name: "by artist album", mode: navBrowseModeByArtistAlbum},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				providers: []ProviderEntry{
					{Key: "navidrome", Provider: sessionProviderStub{key: "navidrome"}},
				},
				navBrowser: navBrowserState{
					mode:     tt.mode,
					prov:     sessionProviderStub{key: "navidrome"},
					selAlbum: provider.AlbumInfo{ID: "album-1", Name: "Album"},
				},
			}

			got := m.navTrackListSource()
			want := source.Ref{
				ProviderKey: "navidrome",
				Kind:        source.Album,
				ID:          "album-1",
			}
			if got != want {
				t.Fatalf("navTrackListSource() = %+v, want %+v", got, want)
			}
		})
	}
}
