package playlist

import "testing"

func TestTrackDisplayName(t *testing.T) {
	tests := []struct {
		name  string
		track Track
		want  string
	}{
		{
			name:  "artist and title",
			track: Track{Artist: "Radiohead", Title: "Creep"},
			want:  "Radiohead - Creep",
		},
		{
			name:  "title only",
			track: Track{Title: "Unknown Song"},
			want:  "Unknown Song",
		},
		{
			name:  "empty",
			track: Track{},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.track.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrackIsLive(t *testing.T) {
	t.Run("realtime", func(t *testing.T) {
		tr := Track{Realtime: true}
		if !tr.IsLive() {
			t.Error("IsLive() = false, want true")
		}
	})

	t.Run("not realtime", func(t *testing.T) {
		tr := Track{Realtime: false}
		if tr.IsLive() {
			t.Error("IsLive() = true, want false")
		}
	})
}

func TestTrackFromURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantTtl string
		stream  bool
	}{
		{
			name:    "with filename",
			url:     "https://example.com/music/song.mp3",
			wantTtl: "song",
			stream:  true,
		},
		{
			name:    "stream path fallback to hostname",
			url:     "https://radio.example.com/stream",
			wantTtl: "radio.example.com",
			stream:  true,
		},
		{
			name:    "rest path fallback to hostname",
			url:     "https://api.example.com/rest",
			wantTtl: "api.example.com",
			stream:  true,
		},
		{
			name:    "query params ignored",
			url:     "https://example.com/song.mp3?token=abc",
			wantTtl: "song",
			stream:  true,
		},
		{
			name:    "root path fallback to hostname",
			url:     "https://radio.example.com/",
			wantTtl: "radio.example.com",
			stream:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := TrackFromPath(tt.url)
			if tr.Title != tt.wantTtl {
				t.Errorf("Title = %q, want %q", tr.Title, tt.wantTtl)
			}
			if tr.Stream != tt.stream {
				t.Errorf("Stream = %v, want %v", tr.Stream, tt.stream)
			}
			if tr.Path != tt.url {
				t.Errorf("Path = %q, want %q", tr.Path, tt.url)
			}
		})
	}
}

func TestTrackFromPathYouTubeInfersArtwork(t *testing.T) {
	tr := TrackFromPath("https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if tr.Artwork.URL != "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg" {
		t.Fatalf("Artwork.URL = %q, want youtube thumbnail", tr.Artwork.URL)
	}
	if tr.Artwork.CacheKey != "youtube:dQw4w9WgXcQ" {
		t.Fatalf("Artwork.CacheKey = %q, want youtube cache key", tr.Artwork.CacheKey)
	}
}

func TestTrackFromPathShortYouTubeInfersArtwork(t *testing.T) {
	tr := TrackFromPath("https://youtu.be/dQw4w9WgXcQ")
	if tr.Artwork.URL != "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg" {
		t.Fatalf("Artwork.URL = %q, want youtube thumbnail", tr.Artwork.URL)
	}
	if tr.Artwork.CacheKey != "youtube:dQw4w9WgXcQ" {
		t.Fatalf("Artwork.CacheKey = %q, want youtube cache key", tr.Artwork.CacheKey)
	}
}

func TestTrackFromPathSpotifySkipsEmbeddedArtwork(t *testing.T) {
	tr := TrackFromPath("spotify:track:abc123")
	if tr.Path != "spotify:track:abc123" {
		t.Fatalf("Path = %q, want spotify URI", tr.Path)
	}
	if tr.Stream {
		t.Fatal("Stream = true, want false")
	}
	if !tr.Artwork.IsNone() {
		t.Fatalf("Artwork = %+v, want none", tr.Artwork)
	}
}

func TestTrackFromPathSSHSkipsEmbeddedArtwork(t *testing.T) {
	tr := TrackFromPath("ssh://nas/music/Artist - Song.mp3")
	if tr.Path != "ssh://nas/music/Artist - Song.mp3" {
		t.Fatalf("Path = %q, want ssh URI", tr.Path)
	}
	if tr.Artist != "Artist" || tr.Title != "Song" {
		t.Fatalf("parsed track = %+v, want Artist - Song", tr)
	}
	if tr.Stream {
		t.Fatal("Stream = true, want false")
	}
	if !tr.Artwork.IsNone() {
		t.Fatalf("Artwork = %+v, want none", tr.Artwork)
	}
}

func TestTrackFromPathColonFilenameKeepsEmbeddedArtwork(t *testing.T) {
	tr := TrackFromPath("foo:bar.mp3")
	if tr.Path != "foo:bar.mp3" {
		t.Fatalf("Path = %q, want colon filename", tr.Path)
	}
	if tr.Title != "foo:bar" {
		t.Fatalf("Title = %q, want %q", tr.Title, "foo:bar")
	}
	if tr.Artwork.Path != "foo:bar.mp3" {
		t.Fatalf("Artwork.Path = %q, want %q", tr.Artwork.Path, "foo:bar.mp3")
	}
}

func TestTrackFromPathColonTitleKeepsEmbeddedArtwork(t *testing.T) {
	tr := TrackFromPath("Artist: Song.mp3")
	if tr.Path != "Artist: Song.mp3" {
		t.Fatalf("Path = %q, want colon filename", tr.Path)
	}
	if tr.Title != "Artist: Song" {
		t.Fatalf("Title = %q, want %q", tr.Title, "Artist: Song")
	}
	if tr.Artwork.Path != "Artist: Song.mp3" {
		t.Fatalf("Artwork.Path = %q, want %q", tr.Artwork.Path, "Artist: Song.mp3")
	}
}

func TestRepeatModeString(t *testing.T) {
	tests := []struct {
		mode RepeatMode
		want string
	}{
		{RepeatOff, "Off"},
		{RepeatAll, "All"},
		{RepeatOne, "One"},
		{RepeatMode(99), "Off"}, // unknown defaults to "Off"
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
