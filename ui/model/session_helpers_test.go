package model

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"cliamp/internal/session"
	"cliamp/internal/source"
	"cliamp/playlist"
	"cliamp/ui"
)

type sessionProviderStub struct {
	key string
}

func (s sessionProviderStub) Name() string                                { return s.key }
func (s sessionProviderStub) Playlists() ([]playlist.PlaylistInfo, error) { return nil, nil }
func (s sessionProviderStub) Tracks(string) ([]playlist.Track, error)     { return nil, nil }

type restoreProviderStub struct {
	sessionProviderStub
}

type restoreRadioProviderStub struct {
	sessionProviderStub
}

type providerBrowserStub struct {
	sessionProviderStub
	playlistsCalled int
	restoreCalled   int
}

func (s *providerBrowserStub) Playlists() ([]playlist.PlaylistInfo, error) {
	s.playlistsCalled++
	return []playlist.PlaylistInfo{{ID: "mix", Name: "Mix"}}, nil
}

func (s *providerBrowserStub) RestoreSource(source.Ref) ([]playlist.Track, error) {
	s.restoreCalled++
	return []playlist.Track{{Title: "restored", Path: "/restored.mp3"}}, nil
}

func (s *restoreProviderStub) RestoreSource(source.Ref) ([]playlist.Track, error) {
	return []playlist.Track{{Title: "restored", Path: "/restored.mp3"}}, nil
}

func (s *restoreProviderStub) ResumeMetaKey() string {
	return s.key + ".id"
}

func (s *restoreRadioProviderStub) RestoreSource(source source.Ref) ([]playlist.Track, error) {
	return []playlist.Track{{Title: "restored", Path: source.ID}}, nil
}

func (s *restoreRadioProviderStub) ResumeMetaKey() string {
	return s.key + ".id"
}

func sessionPlannerFor(entries ...ProviderEntry) session.Planner {
	providers := make([]session.RuntimeProvider, 0, len(entries))
	for _, entry := range entries {
		providers = append(providers, session.RuntimeProvider{
			Key:      entry.Key,
			Provider: entry.Provider,
		})
	}
	return session.NewPlanner(providers)
}

func assertEmptySessionState(t *testing.T, state session.State) {
	t.Helper()
	if state.PositionSec != 0 || state.Source.Valid() {
		t.Fatalf("session state = %+v, want empty state", state)
	}
	if state.Playlist.Current != (session.TrackRef{}) || state.Playlist.Cursor != (session.TrackRef{}) {
		t.Fatalf("playlist refs = %+v, want empty refs", state.Playlist)
	}
	if state.Playlist.CurrentQueued || len(state.Playlist.Queue) != 0 || len(state.Playlist.Order) != 0 {
		t.Fatalf("playlist state = %+v, want empty playlist state", state.Playlist)
	}
}

func TestStaleSourceRestoreTracksMsgIsIgnored(t *testing.T) {
	pl := playlist.New()
	pl.Add(playlist.Track{Title: "existing", Path: "/existing.mp3"})

	m := Model{
		playlist: pl,
		restore: pendingRestore{
			token: 2,
		},
		source: source.Ref{
			ProviderKey: "radio",
			Kind:        source.Playlist,
			ID:          "existing",
		},
	}

	updated, cmd := m.Update(sourceRestoreTracksMsg{
		token: 1,
		result: session.RestoreResult{
			Tracks: []playlist.Track{
				{Title: "restored", Path: "/restored.mp3"},
			},
		},
	})
	if cmd != nil {
		t.Fatal("Update() cmd should be nil for stale restore")
	}

	nextModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update() model = %T, want model.Model", updated)
	}
	if nextModel.playlist.Len() != 1 {
		t.Fatalf("playlist len = %d, want 1", nextModel.playlist.Len())
	}
	current, idx := nextModel.playlist.Current()
	if idx != 0 || current.Path != "/existing.mp3" {
		t.Fatalf("Current() = (%q, %d), want (/existing.mp3, 0)", current.Path, idx)
	}
	if nextModel.source.ID != "existing" {
		t.Fatalf("source ID = %q, want %q", nextModel.source.ID, "existing")
	}
}

func TestSourceRestoreTracksMsgPreservesSavedRadioTitle(t *testing.T) {
	m := Model{
		player:    &fakeEngine{},
		playlist:  playlist.New(),
		vis:       ui.NewVisualizer(44100),
		plVisible: 5,
		restore: pendingRestore{
			token: 1,
			plan: session.RestorePlan{
				Mode: session.RestoreDeferred,
				State: session.State{
					Source: source.Ref{
						ProviderKey: "radio",
						Kind:        source.Playlist,
						ID:          "u:https%3A%2F%2Fdancewave.online%2Fstream",
					},
					Playlist: session.PlaylistState{
						Current: session.TrackRef{
							Index: 0,
							Path:  "https://dancewave.online/stream",
							Title: "Dance Wave!",
						},
					},
				},
			},
		},
	}

	updated, _ := m.Update(sourceRestoreTracksMsg{
		token: 1,
		result: session.RestoreResult{
			Tracks: []playlist.Track{{
				Path:     "https://dancewave.online/stream",
				Title:    "dancewave.online",
				Stream:   true,
				Realtime: true,
			}},
		},
	})

	nextModel := updated.(Model)
	current, idx := nextModel.playlist.Current()
	if idx != 0 {
		t.Fatalf("current index = %d, want 0", idx)
	}
	if current.Title != "Dance Wave!" {
		t.Fatalf("current title = %q, want saved station title", current.Title)
	}
}

func TestCaptureCurrentProviderStateUsesSessionOwnerKey(t *testing.T) {
	pl := playlist.New()
	pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})

	m := Model{
		player:   &fakeEngine{pos: 12 * time.Second},
		playlist: pl,
		provider: sessionProviderStub{key: "radio"},
		providers: []ProviderEntry{
			{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
			{Key: "navidrome", Provider: &restoreProviderStub{sessionProviderStub: sessionProviderStub{key: "navidrome"}}},
		},
		source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "saved-id",
		},
		sessionPlanner: sessionPlannerFor(
			ProviderEntry{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
			ProviderEntry{Key: "navidrome", Provider: &restoreProviderStub{sessionProviderStub: sessionProviderStub{key: "navidrome"}}},
		),
		providerSessions: make(map[string]session.HydratedState),
	}

	m.captureCurrentProviderState()

	cached, ok := m.providerSessions["navidrome"]
	if !ok {
		t.Fatal("missing state for session owner key")
	}
	if cached.State.Source.ProviderKey != "navidrome" || cached.State.Playlist.Current.Path != "/saved.mp3" {
		t.Fatalf("cached state = %+v, want saved source-aware state", cached.State)
	}
	if _, ok := m.providerSessions["navidrome"]; !ok {
		t.Fatalf("providerSessions = %+v, want owner provider entry", m.providerSessions)
	}
}

func TestCaptureCurrentProviderStateKeepsMixedSessionOwner(t *testing.T) {
	pl := playlist.New()
	pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})

	m := Model{
		player:          &fakeEngine{pos: 12 * time.Second},
		playlist:        pl,
		provider:        sessionProviderStub{key: "radio"},
		sessionOwnerKey: "navidrome",
		providers: []ProviderEntry{
			{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
			{Key: "navidrome", Provider: &restoreProviderStub{sessionProviderStub: sessionProviderStub{key: "navidrome"}}},
		},
		sessionPlanner: sessionPlannerFor(
			ProviderEntry{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
			ProviderEntry{Key: "navidrome", Provider: &restoreProviderStub{sessionProviderStub: sessionProviderStub{key: "navidrome"}}},
		),
		providerSessions: make(map[string]session.HydratedState),
	}

	m.captureCurrentProviderState()

	if _, ok := m.providerSessions["navidrome"]; !ok {
		t.Fatalf("providerSessions = %+v, want navidrome owner slot", m.providerSessions)
	}
	if _, ok := m.providerSessions["radio"]; ok {
		t.Fatalf("providerSessions = %+v, want no radio contamination", m.providerSessions)
	}
}

func TestCaptureCurrentProviderStateDoesNotAssignOwnerToProviderlessPlaylist(t *testing.T) {
	pl := playlist.New()
	pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})

	m := Model{
		player:   &fakeEngine{pos: 12 * time.Second},
		playlist: pl,
		provider: sessionProviderStub{key: "radio"},
		providers: []ProviderEntry{
			{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
		},
		sessionPlanner: sessionPlannerFor(
			ProviderEntry{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
		),
		providerSessions: make(map[string]session.HydratedState),
	}

	m.captureCurrentProviderState()

	if _, ok := m.providerSessions["radio"]; ok {
		t.Fatalf("providerSessions = %+v, want no provider slot for ownerless playlist", m.providerSessions)
	}
	if len(m.providerSessions) != 0 {
		t.Fatalf("providerSessions = %+v, want no cached owner entry for ownerless playlist", m.providerSessions)
	}
}

func TestCaptureCurrentProviderStateSkipsStoppedSourceSession(t *testing.T) {
	pl := playlist.New()
	pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})

	m := Model{
		player:   &fakeEngine{notPlaying: true},
		playlist: pl,
		provider: sessionProviderStub{key: "navidrome"},
		providers: []ProviderEntry{
			{Key: "navidrome", Provider: sessionProviderStub{key: "navidrome"}},
		},
		source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "saved-id",
		},
		sessionPlanner: sessionPlannerFor(
			ProviderEntry{Key: "navidrome", Provider: &restoreProviderStub{sessionProviderStub: sessionProviderStub{key: "navidrome"}}},
		),
		providerSessions: make(map[string]session.HydratedState),
	}

	m.captureCurrentProviderState()

	if len(m.providerSessions) != 0 {
		t.Fatalf("providerSessions = %+v, want stopped source session skipped", m.providerSessions)
	}
}

func TestCurrentSessionStateKeepsResumeMetaWhenSourceProviderUnavailable(t *testing.T) {
	pl := playlist.New()
	pl.Add(playlist.Track{
		Title: "saved",
		Path:  "https://nav.example/rest/stream?id=song-1&s=old&t=old",
		ProviderMeta: map[string]string{
			"navidrome.id": "song-1",
		},
	})

	m := Model{
		player:   &fakeEngine{pos: 37 * time.Second},
		playlist: pl,
		source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "saved-id",
		},
		sessionPlanner: session.NewPlanner([]session.RuntimeProvider{
			{Key: "navidrome"},
		}),
	}

	state := m.currentSessionState()
	if !state.IsPlaylistSession() {
		t.Fatal("currentSessionState() should be a playlist session")
	}
	if state.Playlist.Current.MetaKey != "navidrome.id" || state.Playlist.Current.MetaValue != "song-1" {
		t.Fatalf("currentSessionState() current ref = %+v, want durable navidrome id", state.Playlist.Current)
	}
	if state.Playlist.Cursor.MetaKey != "navidrome.id" || state.Playlist.Cursor.MetaValue != "song-1" {
		t.Fatalf("currentSessionState() cursor ref = %+v, want durable navidrome id", state.Playlist.Cursor)
	}
}

func TestSwitchProviderDoesNotRestartCurrentSession(t *testing.T) {
	tests := []struct {
		name      string
		switchIdx int
		model     func(*fakeEngine) Model
	}{
		{
			name:      "active provider",
			switchIdx: 0,
			model: func(engine *fakeEngine) Model {
				pl := playlist.New()
				pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})
				return Model{
					player:   engine,
					playlist: pl,
					provider: sessionProviderStub{key: "navidrome"},
					providers: []ProviderEntry{
						{Key: "navidrome", Provider: sessionProviderStub{key: "navidrome"}},
					},
					sessionPlanner: sessionPlannerFor(
						ProviderEntry{Key: "navidrome", Provider: sessionProviderStub{key: "navidrome"}},
					),
					source: source.Ref{
						ProviderKey: "navidrome",
						Kind:        source.Playlist,
						ID:          "saved-id",
					},
					providerSessions: map[string]session.HydratedState{
						"navidrome": {
							State: session.State{
								Source: source.Ref{
									ProviderKey: "navidrome",
									Kind:        source.Playlist,
									ID:          "stale-id",
								},
							},
							Tracks: []playlist.Track{{Title: "stale", Path: "/stale.mp3"}},
						},
					},
				}
			},
		},
		{
			name:      "source owner",
			switchIdx: 1,
			model: func(engine *fakeEngine) Model {
				pl := playlist.New()
				pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})
				return Model{
					player:   engine,
					playlist: pl,
					provider: sessionProviderStub{key: "radio"},
					providers: []ProviderEntry{
						{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
						{Key: "navidrome", Provider: sessionProviderStub{key: "navidrome"}},
					},
					sessionPlanner: sessionPlannerFor(
						ProviderEntry{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
						ProviderEntry{Key: "navidrome", Provider: sessionProviderStub{key: "navidrome"}},
					),
					source: source.Ref{
						ProviderKey: "navidrome",
						Kind:        source.Playlist,
						ID:          "saved-id",
					},
					providerSessions: map[string]session.HydratedState{
						"navidrome": {
							State: session.State{
								Source: source.Ref{
									ProviderKey: "navidrome",
									Kind:        source.Playlist,
									ID:          "stale-id",
								},
							},
							Tracks: []playlist.Track{{Title: "stale", Path: "/stale.mp3"}},
						},
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &fakeEngine{}
			m := tt.model(engine)

			_ = m.switchProvider(tt.switchIdx)

			if engine.stopCalls != 0 {
				t.Fatalf("Stop() calls = %d, want no restart for current session", engine.stopCalls)
			}
		})
	}
}

func TestCurrentSessionState(t *testing.T) {
	tests := []struct {
		name  string
		model func() Model
		check func(*testing.T, session.State)
	}{
		{
			name: "providerless track becomes playlist session",
			model: func() Model {
				pl := playlist.New()
				pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})
				return Model{
					player:   &fakeEngine{pos: 37 * time.Second},
					playlist: pl,
				}
			},
			check: func(t *testing.T, state session.State) {
				t.Helper()
				if !state.IsPlaylistSession() || state.Playlist.Current.Path != "/saved.mp3" || state.PositionSec != 37 {
					t.Fatalf("currentSessionState() = %+v, want playlist-backed saved track", state)
				}
				if state.OwnerKey != "" {
					t.Fatalf("currentSessionState() owner = %q, want empty owner for providerless playlist", state.OwnerKey)
				}
				if len(state.Tracks) != 1 || state.Tracks[0].Path != "/saved.mp3" {
					t.Fatalf("currentSessionState() tracks = %+v, want saved playlist snapshot", state.Tracks)
				}
			},
		},
		{
			name: "live source",
			model: func() Model {
				pl := playlist.New()
				pl.Add(playlist.Track{
					Title:    "Radio Example",
					Path:     "https://radio.example.com/stream",
					Stream:   true,
					Realtime: true,
				})

				return Model{
					player:   &fakeEngine{pos: 37 * time.Second},
					playlist: pl,
					providers: []ProviderEntry{
						{
							Key:      "radio",
							Provider: &restoreRadioProviderStub{sessionProviderStub: sessionProviderStub{key: "radio"}},
						},
					},
					sessionPlanner: sessionPlannerFor(
						ProviderEntry{
							Key:      "radio",
							Provider: &restoreRadioProviderStub{sessionProviderStub: sessionProviderStub{key: "radio"}},
						},
					),
					source: source.Ref{
						ProviderKey: "radio",
						Kind:        source.Playlist,
						ID:          "u:https%3A%2F%2Fradio.example.com%2Fstream",
					},
				}
			},
			check: func(t *testing.T, state session.State) {
				t.Helper()
				if state.Source.ID != "u:https%3A%2F%2Fradio.example.com%2Fstream" {
					t.Fatalf("saved source ID = %q, want URL-based source ID", state.Source.ID)
				}
				if state.PositionSec != 0 {
					t.Fatalf("saved position = %d, want 0 for live source", state.PositionSec)
				}
				if state.Playlist.Current.Path != "https://radio.example.com/stream" {
					t.Fatalf("saved playlist current = %+v, want live stream track", state.Playlist.Current)
				}
				if len(state.Tracks) != 0 {
					t.Fatalf("saved tracks = %+v, want no persisted source snapshot", state.Tracks)
				}
			},
		},
		{
			name: "built-in radio stream",
			model: func() Model {
				pl := playlist.New()
				pl.Add(playlist.Track{
					Title:    "Lofi Stream",
					Path:     "http://radio.cliamp.stream/lofi/stream",
					Stream:   true,
					Realtime: true,
				})

				prov := &restoreRadioProviderStub{sessionProviderStub: sessionProviderStub{key: "radio"}}
				return Model{
					player:   &fakeEngine{pos: 37 * time.Second},
					playlist: pl,
					provider: prov,
					providers: []ProviderEntry{
						{Key: "radio", Provider: prov},
					},
					sessionPlanner: sessionPlannerFor(
						ProviderEntry{Key: "radio", Provider: prov},
					),
				}
			},
			check: func(t *testing.T, state session.State) {
				t.Helper()
				if state.Source.ID != "u:http%3A%2F%2Fradio.cliamp.stream%2Flofi%2Fstream" {
					t.Fatalf("saved source ID = %q, want URL-based source ID", state.Source.ID)
				}
				if state.PositionSec != 0 {
					t.Fatalf("saved position = %d, want 0 for live source", state.PositionSec)
				}
				if state.Playlist.Current.Path != "http://radio.cliamp.stream/lofi/stream" {
					t.Fatalf("saved playlist current = %+v, want live stream track", state.Playlist.Current)
				}
				if len(state.Tracks) != 0 {
					t.Fatalf("saved tracks = %+v, want no persisted source snapshot", state.Tracks)
				}
			},
		},
		{
			name: "other source-less radio stream becomes playlist session",
			model: func() Model {
				pl := playlist.New()
				pl.Add(playlist.Track{
					Title:  "Other Stream",
					Path:   "https://radio.example.com/stream",
					Stream: true,
				})

				prov := sessionProviderStub{key: "radio"}
				return Model{
					player:   &fakeEngine{pos: 37 * time.Second},
					playlist: pl,
					provider: prov,
					providers: []ProviderEntry{
						{Key: "radio", Provider: prov},
					},
				}
			},
			check: func(t *testing.T, state session.State) {
				t.Helper()
				if !state.IsPlaylistSession() || state.Playlist.Current.Path != "https://radio.example.com/stream" {
					t.Fatalf("session state = %+v, want playlist session", state)
				}
			},
		},
		{
			name: "named local playlist",
			model: func() Model {
				pl := playlist.New()
				pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})
				return Model{
					player:   &fakeEngine{pos: 37 * time.Second},
					playlist: pl,
					providers: []ProviderEntry{
						{Key: "local", Provider: &restoreProviderStub{sessionProviderStub: sessionProviderStub{key: "local"}}},
					},
					sessionPlanner: sessionPlannerFor(
						ProviderEntry{Key: "local", Provider: &restoreProviderStub{sessionProviderStub: sessionProviderStub{key: "local"}}},
					),
					source: source.Ref{
						ProviderKey: "local",
						Kind:        source.Playlist,
						ID:          "mix",
					},
				}
			},
			check: func(t *testing.T, state session.State) {
				t.Helper()
				if !state.IsSourceSession() {
					t.Fatalf("currentSessionState() = %+v, want source session", state)
				}
				if state.Source != (source.Ref{
					ProviderKey: "local",
					Kind:        source.Playlist,
					ID:          "mix",
				}) {
					t.Fatalf("currentSessionState() source = %+v, want named local playlist source", state.Source)
				}
				if state.PositionSec != 37 {
					t.Fatalf("currentSessionState() position = %d, want 37", state.PositionSec)
				}
				if state.Playlist.Current.Path != "/saved.mp3" {
					t.Fatalf("currentSessionState() current = %+v, want saved local track", state.Playlist.Current)
				}
				if len(state.Tracks) != 0 {
					t.Fatalf("currentSessionState() tracks = %+v, want no persisted source snapshot", state.Tracks)
				}
			},
		},
		{
			name: "stopped source session",
			model: func() Model {
				pl := playlist.New()
				pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})
				return Model{
					player:   &fakeEngine{notPlaying: true, pos: 37 * time.Second},
					playlist: pl,
					source: source.Ref{
						ProviderKey: "navidrome",
						Kind:        source.Playlist,
						ID:          "saved-id",
					},
				}
			},
			check: func(t *testing.T, state session.State) {
				t.Helper()
				assertEmptySessionState(t, state)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model()
			tt.check(t, m.currentSessionState())
		})
	}
}

func TestRestoreSessionWithConfiguredLocalTracks(t *testing.T) {
	tracks := []playlist.Track{
		{Title: "A", Path: "/a.mp3"},
		{Title: "B", Path: "/b.mp3"},
		{Title: "C", Path: "/c.mp3"},
	}
	state := session.State{
		PositionSec: 37,
		Source: source.Ref{
			ProviderKey: "local",
			Kind:        source.Playlist,
			ID:          "mix",
		},
		Playlist: session.PlaylistState{
			Current: session.TrackRefFromTrack("", tracks[2], 2),
			Cursor:  session.TrackRefFromTrack("", tracks[2], 2),
			Queue: []session.TrackRef{
				session.TrackRefFromTrack("", tracks[1], 1),
			},
			Order: []session.TrackRef{
				session.TrackRefFromTrack("", tracks[2], 2),
				session.TrackRefFromTrack("", tracks[0], 0),
				session.TrackRefFromTrack("", tracks[1], 1),
			},
		},
	}

	m := Model{
		player:    &fakeEngine{},
		playlist:  playlist.New(),
		vis:       ui.NewVisualizer(44100),
		plVisible: 5,
	}
	if err := m.RestoreSessionWithTracks(state, tracks); err != nil {
		t.Fatalf("RestoreSessionWithTracks: %v", err)
	}

	if m.source != state.Source {
		t.Fatalf("source = %+v, want %+v", m.source, state.Source)
	}
	if m.loadedPlaylist != "mix" {
		t.Fatalf("loadedPlaylist = %q, want mix", m.loadedPlaylist)
	}
	if m.resume.path != "/c.mp3" || m.resume.secs != 37 {
		t.Fatalf("resume = (%q, %d), want (/c.mp3, 37)", m.resume.path, m.resume.secs)
	}
	current, idx := m.playlist.Current()
	if idx != 2 || current.Path != "/c.mp3" {
		t.Fatalf("Current() = (%q, %d), want (/c.mp3, 2)", current.Path, idx)
	}
	if m.playlist.QueuePosition(1) != 1 {
		t.Fatalf("QueuePosition(1) = %d, want 1", m.playlist.QueuePosition(1))
	}
	next, ok := m.playlist.Next()
	if !ok || next.Path != "/b.mp3" {
		t.Fatalf("Next() = (%q, %v), want (/b.mp3, true)", next.Path, ok)
	}
}

func TestResumePlaylistClearsStaleResume(t *testing.T) {
	m := Model{
		player:   &fakeEngine{},
		playlist: playlist.New(),
	}

	m.SetResume("/saved.mp3", 37)
	m.ResumePlaylist("mix", []playlist.Track{
		{Title: "saved", Path: "/saved.mp3"},
		{Title: "other", Path: "/other.mp3"},
	})

	if m.resume.path != "" || m.resume.secs != 0 {
		t.Fatalf("resume = (%q, %d), want cleared", m.resume.path, m.resume.secs)
	}
}

func TestQuitClosesPlayerAndCapturesExitState(t *testing.T) {
	pl := playlist.New()
	pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})
	engine := &fakeEngine{pos: 37 * time.Second}
	m := Model{
		player:   engine,
		playlist: pl,
	}

	cmd := m.quit()
	if cmd == nil {
		t.Fatal("quit() cmd = nil, want tea.Quit")
	}
	if !engine.closed {
		t.Fatal("quit() did not close player")
	}
	if m.exitResume.Playlist.Current.Path != "/saved.mp3" || m.exitResume.PositionSec != 37 {
		t.Fatalf("exitResume = %+v, want saved path and position", m.exitResume)
	}
}

func TestProviderSessionsReturnsPersistableSessions(t *testing.T) {
	m := Model{
		providerSessions: map[string]session.HydratedState{
			"local": {State: session.State{
				Source: source.Ref{
					ProviderKey: "local",
					Kind:        source.Playlist,
					ID:          "mix",
				},
			}},
			"radio": {State: session.State{}},
		},
	}

	got := m.ProviderSessions()
	if len(got) != 1 {
		t.Fatalf("ProviderSessions() len = %d, want 1", len(got))
	}
	if _, ok := got["local"]; !ok {
		t.Fatalf("ProviderSessions() = %+v, want local session", got)
	}
}

func TestProviderSessionsPrefersExitResumeForOwnerSlot(t *testing.T) {
	m := Model{
		providerSessions: map[string]session.HydratedState{
			"navidrome": {State: session.State{
				OwnerKey: "navidrome",
				Tracks:   []playlist.Track{{Path: "/old.mp3"}},
			}},
		},
		exitResume: session.State{
			OwnerKey: "navidrome",
			Tracks:   []playlist.Track{{Path: "/new.mp3"}},
		},
	}

	got := m.ProviderSessions()
	if got["navidrome"].Tracks[0].Path != "/new.mp3" {
		t.Fatalf("ProviderSessions() = %+v, want exit resume in owner slot", got)
	}
}

func TestProviderSessionsSkipsOwnerlessExitResume(t *testing.T) {
	m := Model{
		exitResume: session.State{
			Tracks: []playlist.Track{{Path: "/new.mp3"}},
		},
	}

	if got := m.ProviderSessions(); got != nil {
		t.Fatalf("ProviderSessions() = %+v, want nil for ownerless exit resume", got)
	}
}

func TestLoadPersistedSessionsLoadsCurrentAndProviderSessions(t *testing.T) {
	m := Model{
		player:           &fakeEngine{},
		playlist:         playlist.New(),
		providerSessions: make(map[string]session.HydratedState),
	}

	providerSessions := map[string]session.State{
		"navidrome": {
			Source: source.Ref{
				ProviderKey: "navidrome",
				Kind:        source.Playlist,
				ID:          "mix",
			},
		},
	}

	m.LoadPersistedSessions(providerSessions)

	want := map[string]session.HydratedState{
		"navidrome": {State: providerSessions["navidrome"]},
	}
	if !reflect.DeepEqual(m.providerSessions, want) {
		t.Fatalf("providerSessions = %+v, want %+v", m.providerSessions, want)
	}
}

func TestCaptureCurrentProviderStateDoesNotWarmCacheSyntheticRadioSource(t *testing.T) {
	pl := playlist.New()
	pl.Add(playlist.Track{
		Title:    "Lofi Stream",
		Path:     "http://radio.cliamp.stream/lofi/stream",
		Stream:   true,
		Realtime: true,
	})

	prov := &restoreRadioProviderStub{sessionProviderStub: sessionProviderStub{key: "radio"}}
	m := Model{
		player:   &fakeEngine{pos: 37 * time.Second},
		playlist: pl,
		provider: prov,
		providers: []ProviderEntry{
			{Key: "radio", Provider: prov},
		},
		sessionPlanner: sessionPlannerFor(
			ProviderEntry{Key: "radio", Provider: prov},
		),
		providerSessions: make(map[string]session.HydratedState),
	}

	m.captureCurrentProviderState()

	cached, ok := m.providerSessions["radio"]
	if !ok {
		t.Fatal("missing cached radio state")
	}
	if len(cached.Tracks) != 1 {
		t.Fatalf("cached tracks = %d, want synthetic radio track cached", len(cached.Tracks))
	}
	if cached.State.Source.ID != "u:http%3A%2F%2Fradio.cliamp.stream%2Flofi%2Fstream" {
		t.Fatalf("cached source ID = %q, want URL-based source ID", cached.State.Source.ID)
	}
	if _, ok := m.providerSessions["radio"]; !ok {
		t.Fatalf("providerSessions = %+v, want saved radio session", m.providerSessions)
	}
}

func TestFailedStartupRestoreKeepsSavedProviderState(t *testing.T) {
	m := Model{
		player:           &fakeEngine{},
		playlist:         playlist.New(),
		providerSessions: make(map[string]session.HydratedState),
	}

	saved := session.State{
		PositionSec: 37,
		Source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "saved-id",
		},
	}
	m.LoadPersistedSessions(map[string]session.State{"navidrome": saved})
	m.restore = pendingRestore{
		plan: session.RestorePlan{
			Mode:  session.RestoreDeferred,
			State: saved,
		},
		token: 1,
	}

	updated, cmd := m.Update(sourceRestoreTracksMsg{
		token: m.restore.token,
		result: session.RestoreResult{
			Err: errors.New("offline"),
		},
	})
	if cmd != nil {
		t.Fatal("restore failure cmd should be nil")
	}

	nextModel := updated.(Model)
	all := nextModel.ProviderSessions()
	st, ok := all["navidrome"]
	if !ok {
		t.Fatalf("resume states = %+v, want saved provider state retained", all)
	}
	if st.Source.ID != "saved-id" || st.PositionSec != 37 {
		t.Fatalf("saved provider state = %+v, want original startup state", st)
	}
}

func TestFailedStartupRestoreWithoutSnapshotLeavesPlaylistUntouched(t *testing.T) {
	saved := session.State{
		OwnerKey:    "navidrome",
		PositionSec: 37,
		Source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "saved-id",
		},
		Playlist: session.PlaylistState{
			Current: session.TrackRef{Index: 0, Path: "/saved.mp3"},
			Cursor:  session.TrackRef{Index: 0, Path: "/saved.mp3"},
		},
	}
	m := Model{
		player:   &fakeEngine{},
		playlist: playlist.New(),
		vis:      ui.NewVisualizer(44100),
		restore: pendingRestore{
			plan: session.RestorePlan{
				Mode:  session.RestoreDeferred,
				State: saved,
			},
			token: 1,
		},
	}

	updated, cmd := m.Update(sourceRestoreTracksMsg{
		token: 1,
		result: session.RestoreResult{
			Err: errors.New("offline"),
		},
	})
	if cmd != nil {
		t.Fatal("restore failure cmd should be nil")
	}

	nextModel := updated.(Model)
	if nextModel.playlist.Len() != 0 {
		t.Fatalf("playlist len = %d, want untouched empty playlist", nextModel.playlist.Len())
	}
	if nextModel.sessionOwnerKey != "" {
		t.Fatalf("sessionOwnerKey = %q, want empty owner", nextModel.sessionOwnerKey)
	}
}

func TestSwitchProviderKeepsDeferredStateUntilRestoreSucceeds(t *testing.T) {
	target := &restoreProviderStub{sessionProviderStub: sessionProviderStub{key: "navidrome"}}

	m := Model{
		player:    &fakeEngine{},
		playlist:  playlist.New(),
		vis:       ui.NewVisualizer(44100),
		plVisible: 5,
		provider:  sessionProviderStub{key: "radio"},
		providers: []ProviderEntry{
			{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
			{Key: "navidrome", Provider: target},
		},
		sessionPlanner: sessionPlannerFor(
			ProviderEntry{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
			ProviderEntry{Key: "navidrome", Provider: target},
		),
		providerSessions: map[string]session.HydratedState{
			"navidrome": {
				State: session.State{
					PositionSec: 11,
					Source: source.Ref{
						ProviderKey: "navidrome",
						Kind:        source.Playlist,
						ID:          "saved-id",
					},
				},
			},
		},
	}

	_ = m.switchProvider(1)

	if _, ok := m.providerSessions["navidrome"]; !ok {
		t.Fatal("switchProvider() removed deferred state before restore succeeded")
	}
	if !m.restore.pending() {
		t.Fatal("switchProvider() did not schedule restore from deferred state")
	}
}

func TestSwitchProviderTreatsCurrentSessionOwnerAsSelectingCurrent(t *testing.T) {
	target := &providerBrowserStub{sessionProviderStub: sessionProviderStub{key: "navidrome"}}
	pl := playlist.New()
	pl.Add(playlist.Track{Title: "saved", Path: "/saved.mp3"})

	m := Model{
		player:    &fakeEngine{},
		playlist:  pl,
		vis:       ui.NewVisualizer(44100),
		plVisible: 5,
		provider:  sessionProviderStub{key: "radio"},
		providers: []ProviderEntry{
			{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
			{Key: "navidrome", Provider: target},
		},
		source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "saved-id",
		},
		sessionPlanner: sessionPlannerFor(
			ProviderEntry{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
			ProviderEntry{Key: "navidrome", Provider: target},
		),
	}

	cmd := m.switchProvider(1)

	if cmd == nil {
		t.Fatal("switchProvider() cmd = nil, want provider load cmd")
	}
	if m.restore.pending() {
		t.Fatal("switchProvider() scheduled restore for current session owner")
	}
	if len(m.providerSessions) != 0 {
		t.Fatalf("providerSessions = %+v, want no captured session state", m.providerSessions)
	}
}

func TestSwitchProviderClearsListsAndRestoresPlaylistSession(t *testing.T) {
	target := &providerBrowserStub{sessionProviderStub: sessionProviderStub{key: "navidrome"}}

	m := Model{
		player:    &fakeEngine{},
		playlist:  playlist.New(),
		vis:       ui.NewVisualizer(44100),
		plVisible: 5,
		provider:  sessionProviderStub{key: "radio"},
		providers: []ProviderEntry{
			{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
			{Key: "navidrome", Provider: target},
		},
		providerLists: []playlist.PlaylistInfo{{ID: "stale", Name: "Stale"}},
		providerSessions: map[string]session.HydratedState{
			"navidrome": {State: session.State{
				OwnerKey: "navidrome",
				Tracks:   []playlist.Track{{Title: "saved", Path: "/saved.mp3"}},
				Playlist: session.PlaylistState{
					Current: session.TrackRef{Index: 0, Path: "/saved.mp3"},
					Cursor:  session.TrackRef{Index: 0, Path: "/saved.mp3"},
				},
			}},
		},
		sessionPlanner: sessionPlannerFor(
			ProviderEntry{Key: "radio", Provider: sessionProviderStub{key: "radio"}},
			ProviderEntry{Key: "navidrome", Provider: target},
		),
	}

	cmd := m.switchProvider(1)
	if m.providerLists != nil {
		t.Fatalf("providerLists = %+v, want cleared stale provider lists", m.providerLists)
	}
	if cmd != nil {
		t.Fatalf("switchProvider() cmd = %v, want nil for local playlist restore path", cmd)
	}
	current, idx := m.playlist.Current()
	if idx != 0 || current.Path != "/saved.mp3" {
		t.Fatalf("Current() = (%q, %d), want (/saved.mp3, 0)", current.Path, idx)
	}
	if m.focus != focusPlaylist {
		t.Fatalf("focus = %v, want %v", m.focus, focusPlaylist)
	}
	if target.playlistsCalled != 0 {
		t.Fatalf("Playlists() calls = %d, want 0", target.playlistsCalled)
	}
}

func TestProviderLoadCmdPrefersPendingSourceRestore(t *testing.T) {
	target := &providerBrowserStub{sessionProviderStub: sessionProviderStub{key: "navidrome"}}
	m := Model{
		player:   &fakeEngine{},
		playlist: playlist.New(),
		provider: target,
		providers: []ProviderEntry{
			{Key: "navidrome", Provider: target},
		},
		sessionPlanner: sessionPlannerFor(
			ProviderEntry{Key: "navidrome", Provider: target},
		),
		restore: pendingRestore{
			plan: session.RestorePlan{
				Mode: session.RestoreDeferred,
				State: session.State{
					Source: source.Ref{
						ProviderKey: "navidrome",
						Kind:        source.Playlist,
						ID:          "mix",
					},
				},
			},
			token: 1,
		},
	}

	cmd := m.providerLoadCmd()
	if cmd == nil {
		t.Fatal("providerLoadCmd() = nil, want restore command")
	}
	msg := cmd()
	if _, ok := msg.(sourceRestoreTracksMsg); !ok {
		t.Fatalf("providerLoadCmd() msg = %T, want sourceRestoreTracksMsg", msg)
	}
	if target.restoreCalled != 1 {
		t.Fatalf("RestoreSource() calls = %d, want 1", target.restoreCalled)
	}
	if target.playlistsCalled != 0 {
		t.Fatalf("Playlists() calls = %d, want 0", target.playlistsCalled)
	}
}
