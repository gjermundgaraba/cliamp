package sessionflow

import (
	"cliamp/internal/session"
	"cliamp/internal/source"
)

type StartupPreparation string

const (
	StartupPreparationNone               StartupPreparation = ""
	StartupPreparationConfiguredPlaylist StartupPreparation = "configured_playlist"
	StartupPreparationDefaultRadio       StartupPreparation = "default_radio"
)

type StartupAction string

const (
	StartupActionNone                                  StartupAction = ""
	StartupActionRestoreHydrated                       StartupAction = "restore_hydrated"
	StartupActionRestoreHydratedFromConfiguredPlaylist StartupAction = "restore_hydrated_from_configured_playlist"
	StartupActionRestoreDeferred                       StartupAction = "restore_deferred"
	StartupActionOpenProviderBrowser                   StartupAction = "open_provider_browser"
)

type StartupInput struct {
	Snapshot           session.PersistedSnapshot
	ProviderRefs       []ProviderRef
	ProviderPrefs      ProviderPrefs
	HasExplicitInput   bool
	ConfiguredPlaylist string
	HasLocalProvider   bool
}

type StartupPlan struct {
	Snapshot        session.PersistedSnapshot
	StartupProvider ProviderSelection
	Preparation     StartupPreparation
	Action          StartupAction
	Restore         session.RestorePlan
}

func PlanStartup(planner session.Planner, input StartupInput) StartupPlan {
	sanitized := planner.SanitizeSnapshot(input.Snapshot)
	startupProvider := ResolveStartupProvider(input.ProviderRefs, input.ProviderPrefs)
	loadConfiguredPlaylist := input.ConfiguredPlaylist != "" && input.HasLocalProvider
	plan := StartupPlan{
		Snapshot:        sanitized,
		StartupProvider: startupProvider,
	}

	if loadConfiguredPlaylist {
		plan.Preparation = StartupPreparationConfiguredPlaylist
	}

	if !input.HasExplicitInput {
		switch {
		case loadConfiguredPlaylist:
			if state, ok := startupLocalPlaylistResumeState(sanitized.ProviderSessions, input.ConfiguredPlaylist); ok {
				plan.Restore = session.RestorePlan{
					Mode:  session.RestoreHydratedSnapshot,
					State: state,
				}
				plan.Action = StartupActionRestoreHydratedFromConfiguredPlaylist
			}
		case input.ProviderPrefs.Explicit != "":
			if state, ok := sanitized.ProviderSessions[input.ProviderPrefs.Explicit]; ok {
				plan.Restore = planner.RestorePlanForState(state)
				plan.Action = startupActionForRestore(plan.Restore)
			}
		default:
			state := sanitized.ProviderSessions[sanitized.LastProviderKey]
			plan.Restore = planner.RestorePlanForState(state)
			plan.Action = startupActionForRestore(plan.Restore)
			if state.IsSourceSession() && plan.Restore.Mode == session.RestoreDeferred {
				if chosen, ok := selectAvailableProvider(input.ProviderRefs, state.Source.ProviderKey); ok {
					plan.StartupProvider = chosen
				}
			}
		}
	}

	if plan.Preparation == StartupPreparationNone && plan.Action == StartupActionNone && !input.HasExplicitInput {
		switch {
		case plan.StartupProvider.Key == "radio":
			plan.Preparation = StartupPreparationDefaultRadio
		case plan.StartupProvider.Index >= 0:
			plan.Action = StartupActionOpenProviderBrowser
		}
	}

	return plan
}

func startupActionForRestore(plan session.RestorePlan) StartupAction {
	switch plan.Mode {
	case session.RestoreHydratedSnapshot:
		return StartupActionRestoreHydrated
	case session.RestoreDeferred:
		return StartupActionRestoreDeferred
	default:
		return StartupActionNone
	}
}

func startupLocalPlaylistResumeState(providerSessions map[string]session.State, playlistName string) (session.State, bool) {
	if playlistName == "" {
		return session.State{}, false
	}
	state, ok := providerSessions["local"]
	if !ok {
		return session.State{}, false
	}
	if !state.IsSourceSession() {
		return session.State{}, false
	}
	if state.Source.ProviderKey != "local" || state.Source.Kind != source.Playlist || state.Source.ID != playlistName {
		return session.State{}, false
	}
	return state, true
}
