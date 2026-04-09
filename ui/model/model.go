// Package ui implements the Bubbletea TUI for the CLIAMP terminal music player.
package model

import (
	"time"

	"cliamp/internal/playback"
	"cliamp/internal/session"
	"cliamp/internal/source"
	"cliamp/luaplugin"
	"cliamp/player"
	"cliamp/playlist"
	"cliamp/theme"
	"cliamp/ui"
)

// ConfigSaver persists individual config key-value pairs.
// Satisfied by config.SaveFunc (the default) or a test stub.
type ConfigSaver interface {
	Save(key, value string) error
}

type focusArea int

const (
	focusPlaylist focusArea = iota
	focusEQ
	focusSpeed
	focusProvPill
	focusSearch
	focusProvider
	focusNetSearch
)

func (f focusArea) label() string {
	switch f {
	case focusPlaylist:
		return "Playlist"
	case focusEQ:
		return "Equalizer"
	case focusSpeed:
		return "Speed"
	case focusProvPill:
		return "Source"
	case focusProvider:
		return "Provider"
	case focusSearch:
		return "Search"
	case focusNetSearch:
		return "Online Search"
	default:
		return ""
	}
}

type topLevelScreen int

const (
	screenMain topLevelScreen = iota
	screenKeymap
	screenThemePicker
	screenDevicePicker
	screenFileBrowser
	screenNavBrowser
	screenPlaylistManager
	screenSpotSearch
	screenQueue
	screenInfo
	screenSearch
	screenNetSearch
	screenURLInput
	screenLyrics
	screenJump
	screenFullVisualizer
)

func (s topLevelScreen) hidesVisualizer() bool {
	return s != screenMain && s != screenFullVisualizer
}

// maxPlVisible caps the playlist at a readable height even on tall terminals.
// maxPlExpandVisible is the higher cap used when the user expands with 'x'.
const (
	maxPlVisible       = 12
	maxPlExpandVisible = 24
)

type plMgrScreenType int

const (
	plMgrScreenList plMgrScreenType = iota
	plMgrScreenTracks
	plMgrScreenNewName
)

// navBrowseModeType identifies which Navidrome browse mode is active.
type navBrowseModeType int

const (
	navBrowseModeMenu          navBrowseModeType = iota // top-level mode selector
	navBrowseModeByAlbum                                // paginated album list → track list
	navBrowseModeByArtist                               // artist list → track list (album-separated)
	navBrowseModeByArtistAlbum                          // artist list → album list → track list
)

// navBrowseScreenType identifies which screen within the active browse mode is shown.
type navBrowseScreenType int

const (
	navBrowseScreenList   navBrowseScreenType = iota // first-level list (artists or albums)
	navBrowseScreenAlbums                            // artist's albums (ArtistAlbum mode only)
	navBrowseScreenTracks                            // final song list in any mode
)

// ProviderEntry pairs a display name with a key and provider implementation.
type ProviderEntry struct {
	Key      string            // config key: "radio", "navidrome", "spotify"
	Name     string            // display name: "Radio", "Navidrome", "Spotify"
	Provider playlist.Provider // nil if not configured
}

// statusTTL* constants define how long a status message is shown.
const (
	statusTTLShort   statusTTL = statusTTL(2 * time.Second)         // brief confirmations
	statusTTLDefault statusTTL = statusTTL(3 * time.Second)         // standard status messages
	statusTTLMedium  statusTTL = statusTTL(4 * time.Second)         // messages needing extra visibility
	statusTTLBatch   statusTTL = statusTTL(4500 * time.Millisecond) // batch operation feedback
	statusTTLLong    statusTTL = statusTTL(6 * time.Second)         // loading indicators
)

// minPlVisible is the minimum playlist height when collapsed.
const minPlVisible = 5

// Model is the Bubbletea model for the CLIAMP TUI.
type Model struct {
	// Core playback
	player        player.Engine
	playlist      *playlist.Playlist
	configSaver   ConfigSaver
	vis           *ui.Visualizer
	seekStepLarge time.Duration

	// UI navigation
	focus           focusArea
	prevFocus       focusArea // focus to restore on cancel (search, net search)
	screenBase      topLevelScreen
	screenStack     []topLevelScreen
	eqCursor        int       // selected EQ band (0-9)
	plCursor        int       // selected playlist item
	plScroll        int       // scroll offset for playlist view
	plVisible       int       // desired max visible playlist lines
	titleOff        int       // scroll offset for long track titles
	titleLastScroll time.Time // last time the title scrolled
	err             error
	quitting        bool
	width           int
	height          int

	// Provider state
	provider           playlist.Provider
	activeProviderKey  string
	localProvider      playlist.Provider // local playlist provider for file-based playlist management (always available)
	providerIndexByKey map[string]int
	providerLists      []playlist.PlaylistInfo
	provCursor         int
	provScroll         int
	provLoading        bool
	provSignIn         bool            // true when provider needs interactive sign-in
	providers          []ProviderEntry // all available providers
	provPillIdx        int             // selected pill index; -1 when no provider is selected
	eqPresetIdx        int             // -1 = custom, 0+ = index into eqPresets
	eqCustomLabel      string          // non-empty = plugin-defined preset label (shown instead of "Custom")

	// Overlay / feature state (see state.go for struct definitions)
	search         searchState
	netSearch      netSearchState
	provSearch     provSearchState
	seek           seekState
	themePicker    themePickerState
	lyrics         lyricsState
	keymap         keymapOverlay
	queue          queueOverlay
	plManager      plManagerState
	spotSearch     spotSearchState
	fileBrowser    fileBrowserState
	navBrowser     navBrowserState
	catalogBatch   catalogBatchState
	ytdlBatch      ytdlBatchState
	reconnect      reconnectState
	save           saveState
	status         statusMsg
	logLines       []logLine
	network        networkStats
	speedSaveAfter time.Duration
	termTitle      terminalTitleState

	// Jump to time mode
	jumping   bool
	jumpInput string

	// URL input mode (load playlist/stream URL at runtime)
	urlInputting bool
	urlInput     string

	// Async feed/M3U URL resolution
	pendingURLs []string
	feedLoading bool

	// Async stream buffering (true while HTTP connect is in progress)
	buffering   bool
	bufferingAt time.Time // when buffering started, for elapsed display

	// resume holds the path and position to seek to when the matching track
	// starts playing. Cleared after the seek is performed.
	resume struct {
		path string
		secs int
	}

	loadedPlaylist string // name of the currently loaded local playlist (for resume)

	exitResume session.State

	providerSessions map[string]session.HydratedState

	sessionPlanner session.Planner

	sessionOwnerKey string

	source source.Ref

	pendingSource source.Ref

	restore pendingRestore

	// preloading is true while a preloadStreamCmd goroutine is in-flight.
	preloading bool

	// Live stream title from ICY metadata (e.g., "Artist - Song")
	streamTitle string

	notifier playback.Notifier

	// Lua plugin manager (nil if no plugins loaded)
	luaMgr *luaplugin.Manager

	// Theme state: -1 = Default (ANSI), 0+ = index into themes
	themes   []theme.Theme
	themeIdx int

	// Track info overlay (metadata details)
	showInfo bool

	// Audio device picker overlay
	devicePicker devicePickerState

	// Full-screen visualizer mode (Shift+V)
	fullVis bool

	autoPlay       bool // start playing immediately on launch
	compact        bool // compact mode: cap frame width at 80 columns
	heightExpanded bool // tracks whether manual 'x' expansion is active

	// Cached per-tick to avoid repeated speaker.Lock() calls in View().
	cachedPos  time.Duration
	cachedDur  time.Duration
	lastTickAt time.Time // wall time of previous tickMsg; used for tick delta

}

func (m Model) activeScreen() topLevelScreen {
	if len(m.screenStack) > 0 {
		return m.screenStack[len(m.screenStack)-1]
	}
	if m.screenBase == screenFullVisualizer {
		return screenFullVisualizer
	}
	return screenMain
}

func (m Model) isOverlayActive() bool {
	return m.activeScreen().hidesVisualizer()
}

func (m Model) isPlaying() bool {
	return m.player != nil && m.player.IsPlaying()
}

func (m Model) isPaused() bool {
	return m.player != nil && m.player.IsPaused()
}
