package session

type PersistedSnapshot struct {
	LastProviderKey  string           `json:"last_provider_key,omitempty"`
	ProviderSessions map[string]State `json:"provider_sessions,omitempty"`
}

func (s PersistedSnapshot) Empty() bool {
	return s.LastProviderKey == "" && len(s.ProviderSessions) == 0
}

func CloneState(state State) State {
	if state.IsSourceSession() {
		state.Tracks = nil
		return state
	}
	state.Tracks = CloneTracks(state.Tracks)
	return state
}

func NormalizePersistedSnapshot(snapshot PersistedSnapshot) PersistedSnapshot {
	if len(snapshot.ProviderSessions) > 0 {
		sanitized := make(map[string]State, len(snapshot.ProviderSessions))
		for key, state := range snapshot.ProviderSessions {
			if state.HasPersistableState() {
				sanitized[key] = CloneState(state)
			}
		}
		if len(sanitized) > 0 {
			snapshot.ProviderSessions = sanitized
		} else {
			snapshot.ProviderSessions = nil
		}
	}
	if snapshot.Empty() {
		return PersistedSnapshot{}
	}
	return snapshot
}
