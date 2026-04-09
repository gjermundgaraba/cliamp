package sessionflow

import (
	"testing"

	"cliamp/internal/session"
	"cliamp/internal/source"
	"cliamp/playlist"
)

type plannerProviderStub struct {
	key string
}

func (s plannerProviderStub) Name() string                                { return s.key }
func (s plannerProviderStub) Playlists() ([]playlist.PlaylistInfo, error) { return nil, nil }
func (s plannerProviderStub) Tracks(string) ([]playlist.Track, error)     { return nil, nil }

type plannerRestoreProviderStub struct {
	plannerProviderStub
}

func (s plannerRestoreProviderStub) RestoreSource(source.Ref) ([]playlist.Track, error) {
	return []playlist.Track{{Title: "restored", Path: "/restored.mp3"}}, nil
}

func TestResolveStartupProvider(t *testing.T) {
	providers := []ProviderRef{
		{Key: "radio", Available: true},
		{Key: "navidrome", Available: true},
		{Key: "spotify"},
	}

	tests := []struct {
		name      string
		providers []ProviderRef
		prefs     ProviderPrefs
		want      ProviderSelection
	}{
		{
			name:      "uses explicit provider when available",
			providers: providers,
			prefs: ProviderPrefs{
				Explicit: "navidrome",
				Selected: "radio",
			},
			want: ProviderSelection{Index: 1, Key: "navidrome"},
		},
		{
			name:      "uses selected provider when no explicit provider is set",
			providers: providers,
			prefs: ProviderPrefs{
				Selected: "navidrome",
			},
			want: ProviderSelection{Index: 1, Key: "navidrome"},
		},
		{
			name: "tries selected provider when explicit provider is unavailable",
			providers: []ProviderRef{
				{Key: "radio", Available: true},
				{Key: "spotify", Available: true},
				{Key: "navidrome"},
			},
			prefs: ProviderPrefs{
				Explicit: "navidrome",
				Selected: "spotify",
			},
			want: ProviderSelection{Index: 1, Key: "spotify"},
		},
		{
			name:      "falls back to selected provider when explicit provider is unavailable",
			providers: providers,
			prefs: ProviderPrefs{
				Explicit: "spotify",
				Selected: "navidrome",
			},
			want: ProviderSelection{Index: 1, Key: "navidrome"},
		},
		{
			name:      "falls back to first available provider when saved provider is unavailable",
			providers: providers,
			prefs: ProviderPrefs{
				Selected: "spotify",
			},
			want: ProviderSelection{Index: 0, Key: "radio"},
		},
		{
			name: "returns invalid selection when no providers are available",
			providers: []ProviderRef{
				{Key: "radio"},
				{Key: "navidrome"},
			},
			want: ProviderSelection{Index: -1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveStartupProvider(tt.providers, tt.prefs); got != tt.want {
				t.Fatalf("ResolveStartupProvider() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPlanStartupDropsSavedSourceRestoreWhenProviderIsUnavailable(t *testing.T) {
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "radio", Provider: plannerProviderStub{key: "radio"}},
		{Key: "navidrome"},
	})

	plan := PlanStartup(planner, StartupInput{
		ProviderRefs: []ProviderRef{
			{Key: "radio", Available: true},
			{Key: "navidrome"},
		},
		Snapshot: session.PersistedSnapshot{
			LastProviderKey: "navidrome",
			ProviderSessions: map[string]session.State{
				"navidrome": {
					Source: source.Ref{
						ProviderKey: "navidrome",
						Kind:        source.Playlist,
						ID:          "saved-playlist",
					},
				},
			},
		},
	})

	if plan.Restore.Planned() {
		t.Fatalf("Restore = %+v, want no restore plan", plan.Restore)
	}
	if plan.Action != StartupActionNone {
		t.Fatalf("Action = %q, want %q", plan.Action, StartupActionNone)
	}
	if got := plan.StartupProvider.Key; got != "radio" {
		t.Fatalf("StartupProvider.Key = %q, want radio", got)
	}
	if plan.Preparation != StartupPreparationDefaultRadio {
		t.Fatalf("Preparation = %q, want %q", plan.Preparation, StartupPreparationDefaultRadio)
	}
}

func TestPlanStartupDefersRestoreWhenSavedSourceProviderIsAvailable(t *testing.T) {
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "radio", Provider: plannerProviderStub{key: "radio"}},
		{Key: "navidrome", Provider: plannerRestoreProviderStub{plannerProviderStub: plannerProviderStub{key: "navidrome"}}},
	})

	plan := PlanStartup(planner, StartupInput{
		ProviderRefs: []ProviderRef{
			{Key: "radio", Available: true},
			{Key: "navidrome", Available: true},
		},
		Snapshot: session.PersistedSnapshot{
			LastProviderKey: "navidrome",
			ProviderSessions: map[string]session.State{
				"navidrome": {
					Source: source.Ref{
						ProviderKey: "navidrome",
						Kind:        source.Playlist,
						ID:          "saved-playlist",
					},
				},
			},
		},
	})

	if plan.Restore.Mode != session.RestoreDeferred {
		t.Fatalf("Restore mode = %q, want %q", plan.Restore.Mode, session.RestoreDeferred)
	}
	if plan.Action != StartupActionRestoreDeferred {
		t.Fatalf("Action = %q, want %q", plan.Action, StartupActionRestoreDeferred)
	}
	if got := plan.StartupProvider.Key; got != "navidrome" {
		t.Fatalf("StartupProvider.Key = %q, want navidrome", got)
	}
}

func TestPlanStartupUsesConfiguredPlaylistTracksForLocalResume(t *testing.T) {
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "local", Provider: plannerRestoreProviderStub{plannerProviderStub: plannerProviderStub{key: "local"}}},
	})

	plan := PlanStartup(planner, StartupInput{
		ProviderRefs: []ProviderRef{
			{Key: "local", Available: true},
		},
		Snapshot: session.PersistedSnapshot{
			ProviderSessions: map[string]session.State{
				"local": {
					Source: source.Ref{
						ProviderKey: "local",
						Kind:        source.Playlist,
						ID:          "mix",
					},
				},
			},
		},
		ConfiguredPlaylist: "mix",
		HasLocalProvider:   true,
	})

	if plan.Preparation != StartupPreparationConfiguredPlaylist {
		t.Fatalf("Preparation = %q, want %q", plan.Preparation, StartupPreparationConfiguredPlaylist)
	}
	if plan.Restore.Mode != session.RestoreHydratedSnapshot {
		t.Fatalf("Restore mode = %q, want %q", plan.Restore.Mode, session.RestoreHydratedSnapshot)
	}
	if plan.Action != StartupActionRestoreHydratedFromConfiguredPlaylist {
		t.Fatalf("Action = %q, want %q", plan.Action, StartupActionRestoreHydratedFromConfiguredPlaylist)
	}
}

func TestPlanStartupIgnoresConfiguredPlaylistResumeForWrongLocalSourceKind(t *testing.T) {
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "local", Provider: plannerRestoreProviderStub{plannerProviderStub: plannerProviderStub{key: "local"}}},
	})

	plan := PlanStartup(planner, StartupInput{
		ProviderRefs: []ProviderRef{
			{Key: "local", Available: true},
		},
		Snapshot: session.PersistedSnapshot{
			ProviderSessions: map[string]session.State{
				"local": {
					Source: source.Ref{
						ProviderKey: "local",
						Kind:        source.Album,
						ID:          "mix",
					},
				},
			},
		},
		ConfiguredPlaylist: "mix",
		HasLocalProvider:   true,
	})

	if plan.Preparation != StartupPreparationConfiguredPlaylist {
		t.Fatalf("Preparation = %q, want %q", plan.Preparation, StartupPreparationConfiguredPlaylist)
	}
	if plan.Restore.Planned() {
		t.Fatalf("Restore = %+v, want no restore plan", plan.Restore)
	}
	if plan.Action != StartupActionNone {
		t.Fatalf("Action = %q, want %q", plan.Action, StartupActionNone)
	}
}

func TestPlanStartupLoadsConfiguredPlaylistWithoutRestore(t *testing.T) {
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "local", Provider: plannerRestoreProviderStub{plannerProviderStub: plannerProviderStub{key: "local"}}},
	})

	plan := PlanStartup(planner, StartupInput{
		ProviderRefs: []ProviderRef{
			{Key: "local", Available: true},
		},
		ConfiguredPlaylist: "mix",
		HasLocalProvider:   true,
	})

	if plan.Preparation != StartupPreparationConfiguredPlaylist {
		t.Fatalf("Preparation = %q, want %q", plan.Preparation, StartupPreparationConfiguredPlaylist)
	}
	if plan.Action != StartupActionNone {
		t.Fatalf("Action = %q, want %q", plan.Action, StartupActionNone)
	}
	if plan.Restore.Planned() {
		t.Fatalf("Restore = %+v, want no restore plan", plan.Restore)
	}
}

func TestPlanStartupLoadsDefaultRadioWhenNoRestoreIsAvailable(t *testing.T) {
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "radio", Provider: plannerProviderStub{key: "radio"}},
		{Key: "navidrome", Provider: plannerProviderStub{key: "navidrome"}},
	})

	plan := PlanStartup(planner, StartupInput{
		ProviderRefs: []ProviderRef{
			{Key: "radio", Available: true},
			{Key: "navidrome", Available: true},
		},
		ProviderPrefs: ProviderPrefs{
			Explicit: "radio",
		},
	})

	if plan.Preparation != StartupPreparationDefaultRadio {
		t.Fatalf("Preparation = %q, want %q", plan.Preparation, StartupPreparationDefaultRadio)
	}
	if plan.Action != StartupActionNone {
		t.Fatalf("Action = %q, want %q", plan.Action, StartupActionNone)
	}
}

func TestPlanStartupOpensProviderBrowserWhenProviderSelectedWithoutRestore(t *testing.T) {
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "radio", Provider: plannerProviderStub{key: "radio"}},
		{Key: "navidrome", Provider: plannerProviderStub{key: "navidrome"}},
	})

	plan := PlanStartup(planner, StartupInput{
		ProviderRefs: []ProviderRef{
			{Key: "radio", Available: true},
			{Key: "navidrome", Available: true},
		},
		ProviderPrefs: ProviderPrefs{
			Explicit: "navidrome",
		},
	})

	if plan.Preparation != StartupPreparationNone {
		t.Fatalf("Preparation = %q, want empty", plan.Preparation)
	}
	if plan.Action != StartupActionOpenProviderBrowser {
		t.Fatalf("Action = %q, want %q", plan.Action, StartupActionOpenProviderBrowser)
	}
}

func TestPlanStartupSuppressesBrowserAndRestoreForExplicitInput(t *testing.T) {
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "radio", Provider: plannerProviderStub{key: "radio"}},
		{Key: "navidrome", Provider: plannerProviderStub{key: "navidrome"}},
	})

	plan := PlanStartup(planner, StartupInput{
		ProviderRefs: []ProviderRef{
			{Key: "radio", Available: true},
			{Key: "navidrome", Available: true},
		},
		ProviderPrefs: ProviderPrefs{
			Explicit: "navidrome",
		},
		HasExplicitInput: true,
	})

	if plan.Preparation != StartupPreparationNone {
		t.Fatalf("Preparation = %q, want empty", plan.Preparation)
	}
	if plan.Action != StartupActionNone {
		t.Fatalf("Action = %q, want %q", plan.Action, StartupActionNone)
	}
	if plan.Restore.Planned() {
		t.Fatalf("Restore = %+v, want no restore plan", plan.Restore)
	}
}
