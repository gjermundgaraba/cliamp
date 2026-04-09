package sessionflow

import (
	"testing"

	"cliamp/internal/session"
	"cliamp/internal/source"
	"cliamp/playlist"
)

func TestPlanProviderSwitchNoOpWhenSelectingCurrentProvider(t *testing.T) {
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "radio", Provider: plannerProviderStub{key: "radio"}},
	})

	plan := PlanProviderSwitch(planner, ProviderSwitchInput{
		TargetKey:          "radio",
		CurrentProviderKey: "radio",
		CurrentOwnerKey:    "radio",
	})

	if !plan.SelectingCurrent {
		t.Fatal("SelectingCurrent = false, want true")
	}
	if plan.Action != ProviderSwitchNone {
		t.Fatalf("Action = %q, want %q", plan.Action, ProviderSwitchNone)
	}
}

func TestPlanProviderSwitchNoOpWhenSelectingCurrentOwner(t *testing.T) {
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "navidrome", Provider: plannerProviderStub{key: "navidrome"}},
	})

	plan := PlanProviderSwitch(planner, ProviderSwitchInput{
		TargetKey:          "navidrome",
		CurrentProviderKey: "radio",
		CurrentOwnerKey:    "navidrome",
		Sessions: map[string]session.HydratedState{
			"navidrome": {
				State: session.State{
					Tracks: []playlist.Track{
						{Title: "saved", Path: "/saved.mp3"},
					},
				},
			},
		},
	})

	if !plan.SelectingCurrent {
		t.Fatal("SelectingCurrent = false, want true")
	}
	if plan.Action != ProviderSwitchNone {
		t.Fatalf("Action = %q, want %q", plan.Action, ProviderSwitchNone)
	}
}

func TestPlanProviderSwitchHydratesCachedSessionAndSchedulesRefetch(t *testing.T) {
	sourceRef := source.Ref{
		ProviderKey: "navidrome",
		Kind:        source.Playlist,
		ID:          "mix",
	}
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "navidrome", Provider: plannerRestoreProviderStub{plannerProviderStub: plannerProviderStub{key: "navidrome"}}},
	})

	plan := PlanProviderSwitch(planner, ProviderSwitchInput{
		TargetKey: "navidrome",
		Sessions: map[string]session.HydratedState{
			"navidrome": {
				State: session.State{
					Source: sourceRef,
					Tracks: []playlist.Track{{Title: "saved", Path: "/saved.mp3"}},
				},
				Tracks: []playlist.Track{{Title: "hydrated", Path: "/hydrated.mp3"}},
			},
		},
	})

	if plan.Action != ProviderSwitchRestoreHydrated {
		t.Fatalf("Action = %q, want %q", plan.Action, ProviderSwitchRestoreHydrated)
	}
	if plan.Restore.Mode != session.RestoreHydratedSnapshot {
		t.Fatalf("Restore.Mode = %q, want %q", plan.Restore.Mode, session.RestoreHydratedSnapshot)
	}
	if got := plan.BackgroundRefetchSource; got != sourceRef {
		t.Fatalf("BackgroundRefetchSource = %+v, want %+v", got, sourceRef)
	}
	if len(plan.Restore.Tracks) != 1 || plan.Restore.Tracks[0].Path != "/hydrated.mp3" {
		t.Fatalf("Restore.Tracks = %+v, want hydrated cached tracks", plan.Restore.Tracks)
	}
}

func TestPlanProviderSwitchDefersSourceRestoreFromCachedState(t *testing.T) {
	sourceRef := source.Ref{
		ProviderKey: "navidrome",
		Kind:        source.Playlist,
		ID:          "mix",
	}
	planner := session.NewPlanner([]session.RuntimeProvider{
		{Key: "navidrome", Provider: plannerRestoreProviderStub{plannerProviderStub: plannerProviderStub{key: "navidrome"}}},
	})

	plan := PlanProviderSwitch(planner, ProviderSwitchInput{
		TargetKey: "navidrome",
		Sessions: map[string]session.HydratedState{
			"navidrome": {
				State: session.State{
					Source: sourceRef,
				},
			},
		},
	})

	if plan.Action != ProviderSwitchRestoreDeferred {
		t.Fatalf("Action = %q, want %q", plan.Action, ProviderSwitchRestoreDeferred)
	}
	if plan.Restore.Mode != session.RestoreDeferred {
		t.Fatalf("Restore.Mode = %q, want %q", plan.Restore.Mode, session.RestoreDeferred)
	}
	if plan.BackgroundRefetchSource != (source.Ref{}) {
		t.Fatalf("BackgroundRefetchSource = %+v, want empty source", plan.BackgroundRefetchSource)
	}
}
