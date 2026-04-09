package main

import (
	"errors"
	"testing"

	"cliamp/internal/session"
	"cliamp/internal/sessionflow"
	"cliamp/internal/source"
	"cliamp/playlist"
	"cliamp/ui/model"
)

func TestShouldLoadDefaultRadio(t *testing.T) {
	tests := []struct {
		name             string
		startupProvider  string
		hasExplicitInput bool
		snapshot         session.PersistedSnapshot
		want             bool
	}{
		{
			name:            "loads default radio without startup restore",
			startupProvider: "radio",
			want:            true,
		},
		{
			name:            "suppresses default radio for source restore",
			startupProvider: "radio",
			snapshot: session.PersistedSnapshot{
				ProviderSessions: map[string]session.State{
					"radio": {
						Source: source.Ref{
							ProviderKey: "radio",
							Kind:        source.Playlist,
							ID:          "mix",
						},
					},
				},
			},
		},
		{
			name:             "suppresses default radio when startup content exists",
			startupProvider:  "radio",
			hasExplicitInput: true,
		},
		{
			name:            "suppresses default radio for other providers",
			startupProvider: "navidrome",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planner := session.NewPlanner([]session.RuntimeProvider{
				{Key: "radio", Provider: playlistProviderStub{}},
				{Key: "navidrome", Provider: playlistProviderStub{}},
			})
			plan := sessionflow.PlanStartup(planner, sessionflow.StartupInput{
				Snapshot: tt.snapshot,
				ProviderRefs: []sessionflow.ProviderRef{
					{Key: "radio", Available: true},
					{Key: "navidrome", Available: true},
				},
				ProviderPrefs: sessionflow.ProviderPrefs{
					Explicit: tt.startupProvider,
				},
				HasExplicitInput: tt.hasExplicitInput,
			})
			if got := plan.Preparation == sessionflow.StartupPreparationDefaultRadio; got != tt.want {
				t.Fatalf("default radio content = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartupLocalPlaylistResumeState(t *testing.T) {
	tests := []struct {
		name                        string
		allStates                   map[string]session.State
		playlistName                string
		startupHasOnlyLocalPlaylist bool
		wantOK                      bool
	}{
		{
			name: "matches named local playlist state",
			allStates: map[string]session.State{
				"local": {
					Source: source.Ref{
						ProviderKey: "local",
						Kind:        source.Playlist,
						ID:          "mix",
					},
				},
			},
			playlistName:                "mix",
			startupHasOnlyLocalPlaylist: true,
			wantOK:                      true,
		},
		{
			name: "ignores mismatched playlist name",
			allStates: map[string]session.State{
				"local": {
					Source: source.Ref{
						ProviderKey: "local",
						Kind:        source.Playlist,
						ID:          "other",
					},
				},
			},
			playlistName:                "mix",
			startupHasOnlyLocalPlaylist: true,
		},
		{
			name: "ignores wrong local source kind",
			allStates: map[string]session.State{
				"local": {
					Source: source.Ref{
						ProviderKey: "local",
						Kind:        source.Album,
						ID:          "mix",
					},
				},
			},
			playlistName:                "mix",
			startupHasOnlyLocalPlaylist: true,
		},
		{
			name: "ignores mixed startup input",
			allStates: map[string]session.State{
				"local": {
					Source: source.Ref{
						ProviderKey: "local",
						Kind:        source.Playlist,
						ID:          "mix",
					},
				},
			},
			playlistName: "mix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planner := session.NewPlanner([]session.RuntimeProvider{
				{Key: "local", Provider: playlistProviderStub{}},
			})
			plan := sessionflow.PlanStartup(planner, sessionflow.StartupInput{
				Snapshot: session.PersistedSnapshot{ProviderSessions: tt.allStates},
				ProviderRefs: []sessionflow.ProviderRef{
					{Key: "local", Available: true},
				},
				ProviderPrefs: sessionflow.ProviderPrefs{
					Selected: "local",
				},
				ConfiguredPlaylist: tt.playlistName,
				HasLocalProvider:   true,
				HasExplicitInput:   !tt.startupHasOnlyLocalPlaylist,
			})
			gotOK := plan.Action == sessionflow.StartupActionRestoreHydratedFromConfiguredPlaylist
			if gotOK != tt.wantOK {
				t.Fatalf("configured local restore = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

func TestDefaultRadioTracksAreLive(t *testing.T) {
	tracks := defaultRadioTracks()
	if len(tracks) != 3 {
		t.Fatalf("defaultRadioTracks() returned %d tracks, want 3", len(tracks))
	}
	for _, track := range tracks {
		if !track.Stream {
			t.Fatalf("track %q is not marked as a stream", track.Path)
		}
		if !track.Realtime {
			t.Fatalf("track %q is not marked as realtime", track.Path)
		}
		if !track.IsLive() {
			t.Fatalf("track %q is not treated as live radio", track.Path)
		}
	}
}

type failingRestoreProviderStub struct{}

func (failingRestoreProviderStub) Name() string                                { return "failing" }
func (failingRestoreProviderStub) Playlists() ([]playlist.PlaylistInfo, error) { return nil, nil }
func (failingRestoreProviderStub) Tracks(string) ([]playlist.Track, error)     { return nil, nil }
func (failingRestoreProviderStub) RestoreSource(source.Ref) ([]playlist.Track, error) {
	return nil, errors.New("offline")
}

var _ source.Restorer = failingRestoreProviderStub{}

func TestRestoreSessionReturnsErrorForFailedSourceSessionRestore(t *testing.T) {
	state := session.State{
		OwnerKey: "navidrome",
		Source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "mix",
		},
	}
	providers := []model.ProviderEntry{
		{Key: "navidrome", Provider: failingRestoreProviderStub{}},
	}

	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "navidrome", Provider: providers[0].Provider},
	})
	result := planner.Restore(state)
	if result.Err == nil || result.Err.Error() != "offline" {
		t.Fatalf("Restore() error = %v, want offline", result.Err)
	}
	if len(result.Tracks) != 0 {
		t.Fatalf("Restore() tracks = %+v, want no fallback tracks", result.Tracks)
	}
}

func TestSanitizeLoadedStateDropsUnavailableSourceSession(t *testing.T) {
	state := session.State{
		OwnerKey: "navidrome",
		Source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "mix",
		},
	}

	got := session.NewPlanner(nil).SanitizeState(state)
	if got.HasPersistableState() || got.Source != (source.Ref{}) || len(got.Tracks) != 0 {
		t.Fatalf("SanitizeState() = %+v, want empty state", got)
	}
}

type playlistProviderStub struct{}

func (playlistProviderStub) Name() string                                { return "stub" }
func (playlistProviderStub) Playlists() ([]playlist.PlaylistInfo, error) { return nil, nil }
func (playlistProviderStub) Tracks(string) ([]playlist.Track, error)     { return nil, nil }
func (playlistProviderStub) RestoreSource(source.Ref) ([]playlist.Track, error) {
	return nil, nil
}
