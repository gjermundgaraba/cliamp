package model

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"cliamp/internal/session"
	"cliamp/luaplugin"
	"cliamp/player"
	"cliamp/playlist"
	"cliamp/theme"
	"cliamp/ui"
)

// applyThemeAll updates colors, spectrum styles, and model-specific styles.
func applyThemeAll(t theme.Theme) {
	ui.ApplyThemeColors(t)
	rebuildModelStyles()
}

// New creates a Model wired to the given player and playlist.
// providers is the ordered list of available providers (Radio, Navidrome, Spotify, Jellyfin, etc.).
// localProv is an optional direct reference to the local provider for write ops.
func New(p player.Engine, pl *playlist.Playlist, providers []ProviderEntry, startupProviderIdx int, localProv playlist.Provider, planner session.Planner, themes []theme.Theme, luaMgr *luaplugin.Manager, cs ConfigSaver) Model {
	m := Model{
		player:             p,
		playlist:           pl,
		configSaver:        cs,
		vis:                ui.NewVisualizer(float64(p.SampleRate())),
		seekStepLarge:      30 * time.Second,
		plVisible:          5,
		eqPresetIdx:        -1, // custom until a preset is selected
		screenBase:         screenMain,
		themes:             themes,
		themeIdx:           -1, // Default (ANSI)
		localProvider:      localProv,
		providers:          providers,
		providerIndexByKey: make(map[string]int, len(providers)),
		provPillIdx:        -1,
		navBrowser:         navBrowserState{},
		luaMgr:             luaMgr,
		sessionPlanner:     planner,
		providerSessions:   make(map[string]session.HydratedState),
	}
	for i, providerEntry := range providers {
		m.providerIndexByKey[providerEntry.Key] = i
	}
	m.termTitle = initialTerminalTitleState()
	if startupProviderIdx >= 0 && startupProviderIdx < len(providers) {
		m.provPillIdx = startupProviderIdx
		m.provider = providers[startupProviderIdx].Provider
		m.activeProviderKey = providers[startupProviderIdx].Key
	}
	m.syncTopLevelScreenFlags()
	return m
}

// findProviderWith returns the first registered provider that satisfies the
// given capability check. This is used for cross-provider shortcuts like "N"
// (browse) and "F" (search) which should work regardless of the active provider.
func (m *Model) findProviderWith(check func(playlist.Provider) bool) playlist.Provider {
	// Prefer the active provider if it matches.
	if check(m.provider) {
		return m.provider
	}
	for _, pe := range m.providers {
		if pe.Provider != nil && check(pe.Provider) {
			return pe.Provider
		}
	}
	return nil
}

// SetAutoPlay makes the player start playback immediately on Init.
func (m *Model) SetAutoPlay(v bool) { m.autoPlay = v }

// SetCompact enables compact mode which caps the frame width at 80 columns.
func (m *Model) SetCompact(v bool) { m.compact = v }

// SetSeekStepLarge configures the Shift+Left/Right seek jump amount.
func (m *Model) SetSeekStepLarge(d time.Duration) {
	switch {
	case d <= 0:
		m.seekStepLarge = 30 * time.Second
	case d <= 5*time.Second:
		m.seekStepLarge = 6 * time.Second
	default:
		m.seekStepLarge = d
	}
}

// SetTheme finds a theme by name and applies it. Returns true if found.
func (m *Model) SetTheme(name string) bool {
	if name == "" || strings.EqualFold(name, "default") {
		m.themeIdx = -1
		applyThemeAll(theme.Default())
		return true
	}
	for i, t := range m.themes {
		if strings.EqualFold(t.Name, name) {
			m.themeIdx = i
			applyThemeAll(t)
			return true
		}
	}
	return false
}

// SetVisualizer sets the visualizer mode by name (case-insensitive).
// Returns true if a valid mode name was recognized. Does not modify state
// if the name is not found, matching the SetTheme guard pattern.
func (m *Model) SetVisualizer(name string) bool {
	mode, ok := ui.StringToVisModeExact(name)
	if !ok {
		return false
	}
	m.vis.Mode = mode
	m.vis.RequestRefresh()
	return true
}

// VisualizerName returns the current visualizer mode's display name.
func (m *Model) VisualizerName() string {
	return m.vis.ModeName()
}

// RegisterLuaVisualizers adds Lua visualizer plugins to the visualizer cycle.
func (m *Model) RegisterLuaVisualizers(names []string, renderer ui.LuaVisRenderer) {
	m.vis.RegisterLuaVisualizers(names, renderer)
}

// SetResume registers a path+position to seek to when that track first plays.
func (m *Model) SetResume(path string, secs int) {
	m.resume.path = path
	m.resume.secs = secs
}

func (m *Model) LoadPersistedSessions(providerSessions map[string]session.State) {
	for key, state := range providerSessions {
		if !state.HasPersistableState() {
			continue
		}
		m.providerSessions[key] = session.HydratedState{State: state}
	}
}

func (m *Model) ScheduleStartupRestore(plan session.RestorePlan) {
	if plan.Mode != session.RestoreDeferred || !plan.State.IsSourceSession() {
		return
	}
	m.restore = pendingRestore{
		plan:  plan,
		token: m.restore.token + 1,
	}
}

func (m Model) ActiveProviderKey() string {
	return m.currentProviderKey()
}

func (m *Model) ResumePlaylist(name string, tracks []playlist.Track) {
	m.SetResume("", 0)
	m.replacePlaylistWithSource(tracks, localPlaylistSource(name))
}

func (m *Model) RestoreSessionWithTracks(state session.State, tracks []playlist.Track) error {
	if !state.HasPersistableState() {
		return nil
	}
	return m.primeRestoredSession(state, session.CloneTracks(tracks))
}

func (m *Model) ApplyStartupRestore(plan session.RestorePlan, tracks []playlist.Track) error {
	if !plan.Planned() {
		return nil
	}
	if plan.Mode != session.RestoreHydratedSnapshot {
		return nil
	}
	if len(tracks) == 0 {
		tracks = plan.Tracks
	}
	return m.RestoreSessionWithTracks(plan.State, tracks)
}

func (m Model) ProviderSessions() map[string]session.State {
	sessions := make(map[string]session.State, len(m.providerSessions)+1)
	for key, cached := range m.providerSessions {
		if cached.State.HasPersistableState() {
			sessions[key] = cached.State
		}
	}
	if m.exitResume.HasPersistableState() {
		if ownerKey := m.exitResume.SessionOwnerKey(); ownerKey != "" {
			sessions[ownerKey] = m.exitResume
		}
	}
	if len(sessions) == 0 {
		return nil
	}
	return sessions
}

// ThemeName returns the current theme name.
func (m Model) ThemeName() string {
	if m.themeIdx < 0 || m.themeIdx >= len(m.themes) {
		return theme.DefaultName
	}
	return m.themes[m.themeIdx].Name
}

// Init starts the tick timer and requests the terminal size.
func (m Model) Init() tea.Cmd {
	if m.luaMgr != nil {
		m.luaMgr.Emit(luaplugin.EventAppStart, nil)
	}
	cmds := []tea.Cmd{tickCmd(), func() tea.Msg { return tea.RequestWindowSize() }}
	if m.provider != nil {
		cmds = append(cmds, m.providerLoadCmd())
	}
	if len(m.pendingURLs) > 0 {
		cmds = append(cmds, resolveRemoteCmd(m.pendingURLs, m.autoPlay))
	}
	if m.autoPlay && m.playlist.Len() > 0 {
		cmds = append(cmds, func() tea.Msg { return autoPlayMsg{} })
	}
	return tea.Batch(cmds...)
}
