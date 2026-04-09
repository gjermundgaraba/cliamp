package session

import (
	"errors"

	"cliamp/internal/source"
	"cliamp/playlist"
)

var (
	ErrSavedSourceEmpty         = errors.New("saved source is empty")
	ErrSourceRestoreUnsupported = errors.New("provider does not support restore")
)

type RuntimeProvider struct {
	Key      string
	Provider playlist.Provider
}

type RestorePlanMode string

const (
	RestoreNone             RestorePlanMode = ""
	RestoreHydratedSnapshot RestorePlanMode = "hydrated_snapshot"
	RestoreDeferred         RestorePlanMode = "deferred"
)

type RestorePlan struct {
	Mode   RestorePlanMode
	State  State
	Tracks []playlist.Track
}

func (p RestorePlan) Planned() bool {
	return p.Mode != RestoreNone && p.State.HasPersistableState()
}

type HydratedState struct {
	State  State
	Tracks []playlist.Track
}

type RestoreResult struct {
	Tracks []playlist.Track
	Err    error
}

type Planner struct {
	providers map[string]playlist.Provider
}

func NewPlanner(providers []RuntimeProvider) Planner {
	lookup := make(map[string]playlist.Provider, len(providers))
	for _, providerRef := range providers {
		if providerRef.Provider != nil {
			lookup[providerRef.Key] = providerRef.Provider
		}
	}
	return Planner{providers: lookup}
}

func (p Planner) Provider(key string) playlist.Provider {
	return p.providers[key]
}

func (p Planner) CanRestoreSource(key string) bool {
	prov := p.Provider(key)
	if prov == nil {
		return false
	}
	_, ok := prov.(source.Restorer)
	return ok
}

func (p Planner) ResumeMetaKey(key string) string {
	prov := p.Provider(key)
	if prov == nil {
		return ""
	}
	matcher, ok := prov.(source.Matcher)
	if !ok {
		return ""
	}
	return matcher.ResumeMetaKey()
}

func (p Planner) SanitizeState(state State) State {
	if !state.HasPersistableState() {
		return State{}
	}
	if state.IsSourceSession() && !p.CanRestoreSource(state.Source.ProviderKey) {
		return State{}
	}
	if len(state.Tracks) > 0 {
		state.Tracks = CloneTracks(state.Tracks)
	}
	return state
}

func (p Planner) SanitizeSnapshot(snapshot PersistedSnapshot) PersistedSnapshot {
	snapshot = NormalizePersistedSnapshot(snapshot)
	if len(snapshot.ProviderSessions) == 0 {
		snapshot.ProviderSessions = nil
		return snapshot
	}
	sanitized := make(map[string]State, len(snapshot.ProviderSessions))
	for key, state := range snapshot.ProviderSessions {
		state = p.SanitizeState(state)
		if state.HasPersistableState() {
			sanitized[key] = state
		}
	}
	if len(sanitized) == 0 {
		snapshot.ProviderSessions = nil
		return snapshot
	}
	snapshot.ProviderSessions = sanitized
	return snapshot
}

func hydratedRestorePlan(state State, tracks []playlist.Track) RestorePlan {
	if !state.HasPersistableState() {
		return RestorePlan{}
	}
	return RestorePlan{
		Mode:   RestoreHydratedSnapshot,
		State:  state,
		Tracks: CloneTracks(tracks),
	}
}

func (p Planner) RestorePlanForState(state State) RestorePlan {
	switch {
	case !state.HasPersistableState():
		return RestorePlan{}
	case state.IsPlaylistSession():
		return hydratedRestorePlan(state, state.Tracks)
	case state.IsSourceSession() && p.CanRestoreSource(state.Source.ProviderKey):
		return RestorePlan{
			Mode:  RestoreDeferred,
			State: state,
		}
	default:
		return RestorePlan{}
	}
}

func (p Planner) RestorePlanForHydratedState(state HydratedState) RestorePlan {
	if len(state.Tracks) > 0 {
		return hydratedRestorePlan(state.State, state.Tracks)
	}
	return p.RestorePlanForState(state.State)
}

func (p Planner) Restore(state State) RestoreResult {
	result := RestoreResult{}
	switch {
	case state.IsPlaylistSession():
		result.Tracks = CloneTracks(state.Tracks)
		return result
	case !state.IsSourceSession():
		result.Err = errors.New("session is not restorable")
		return result
	}

	prov := p.Provider(state.Source.ProviderKey)
	if prov == nil {
		result.Err = errors.New("provider unavailable")
	} else if restorer, ok := prov.(source.Restorer); ok {
		tracks, err := restorer.RestoreSource(state.Source)
		if err == nil && len(tracks) > 0 {
			result.Tracks = tracks
			return result
		}
		if err != nil {
			result.Err = err
		} else {
			result.Err = ErrSavedSourceEmpty
		}
	} else {
		result.Err = ErrSourceRestoreUnsupported
	}
	return result
}
