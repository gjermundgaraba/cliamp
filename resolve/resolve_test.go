package resolve

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestArgsTreatsXiaoyuzhouEpisodeAsPending(t *testing.T) {
	url := "https://www.xiaoyuzhoufm.com/episode/69a13b07a22480add648dd03?s=eyJ1IjogIjYxODEzNmZiZTBmNWU3MjNiYjk2MmE5MiJ9"

	got, err := Args([]string{url})
	if err != nil {
		t.Fatalf("Args returned error: %v", err)
	}
	if len(got.Tracks) != 0 {
		t.Fatalf("Args returned %d immediate tracks, want 0", len(got.Tracks))
	}
	if len(got.Pending) != 1 || got.Pending[0] != url {
		t.Fatalf("Args pending = %#v, want [%q]", got.Pending, url)
	}
}

func TestRemoteResolvesXiaoyuzhouEpisodeHTML(t *testing.T) {
	const episodeURL = "https://www.xiaoyuzhoufm.com/episode/69a13b07a22480add648dd03?s=eyJ1IjogIjYxODEzNmZiZTBmNWU3MjNiYjk2MmE5MiJ9"
	const audioURL = "https://media.xyzcdn.net/65d322815c5cc49b4db454a8/lqbqTgipk04QFSwIMACyGNK655rR.m4a"
	const title = "周轶君对话张艾嘉：我从不刻意标榜“女性”"
	const podcast = "山下声"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/episode/69a13b07a22480add648dd03" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html><head>
<script name="schema:podcast-show" type="application/ld+json">{
  "@context":"https://schema.org/",
  "@type":"PodcastEpisode",
  "url":"https://www.xiaoyuzhoufm.com/episode/69a13b07a22480add648dd03",
  "name":"` + title + `",
  "timeRequired":"PT106M",
  "associatedMedia":{"@type":"MediaObject","contentUrl":"` + audioURL + `"},
  "partOfSeries":{"@type":"PodcastSeries","name":"` + podcast + `","url":"https://www.xiaoyuzhoufm.com/podcast/65d322815c5cc49b4db454a8"}
}</script>
</head><body></body></html>`))
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	oldClient := httpClient
	httpClient = &http.Client{
		Timeout:   30 * time.Second,
		Transport: rewriteHostTransport{target: target, rt: http.DefaultTransport},
	}
	defer func() {
		httpClient = oldClient
	}()

	tracks, err := Remote([]string{episodeURL})
	if err != nil {
		t.Fatalf("Remote returned error: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("Remote returned %d tracks, want 1", len(tracks))
	}
	track := tracks[0]
	if track.Path != audioURL {
		t.Fatalf("track.Path = %q, want %q", track.Path, audioURL)
	}
	if track.Title != title {
		t.Fatalf("track.Title = %q, want %q", track.Title, title)
	}
	if track.Artist != podcast {
		t.Fatalf("track.Artist = %q, want %q", track.Artist, podcast)
	}
	if !track.Stream {
		t.Fatalf("track.Stream = false, want true")
	}
	if track.DurationSecs != 106*60 {
		t.Fatalf("track.DurationSecs = %d, want %d", track.DurationSecs, 106*60)
	}
}

func TestParseXiaoyuzhouOgAudioTakesPrecedence(t *testing.T) {
	const audioURL = "https://media.xyzcdn.net/audio.m4a"
	const title = "Test Episode"

	doc := `<!DOCTYPE html>
<html><head>
<meta property="og:audio" content="` + audioURL + `">
<meta property="og:title" content="` + title + `">
</head><body></body></html>`

	track, err := parseXiaoyuzhouEpisodeHTML("https://www.xiaoyuzhoufm.com/episode/abc", doc)
	if err != nil {
		t.Fatalf("parseXiaoyuzhouEpisodeHTML returned error: %v", err)
	}
	if track.Path != audioURL {
		t.Fatalf("track.Path = %q, want %q", track.Path, audioURL)
	}
	if track.Title != title {
		t.Fatalf("track.Title = %q, want %q", track.Title, title)
	}
}

func TestResolveFeedReusesArtworkCacheKeyForSharedChannelImage(t *testing.T) {
	const imageURL = "https://cdn.example.com/show.png"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Podcast</title>
    <itunes:image href="` + imageURL + `" />
    <item>
      <title>Episode 1</title>
      <enclosure url="https://cdn.example.com/ep1.mp3" type="audio/mpeg" />
    </item>
    <item>
      <title>Episode 2</title>
      <enclosure url="https://cdn.example.com/ep2.mp3" type="audio/mpeg" />
    </item>
  </channel>
</rss>`))
	}))
	defer server.Close()

	tracks, err := resolveFeed(server.URL)
	if err != nil {
		t.Fatalf("resolveFeed() error = %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("resolveFeed() returned %d tracks, want 2", len(tracks))
	}
	if tracks[0].Artwork.CacheKey != "feed:"+imageURL {
		t.Fatalf("first artwork cache key = %q, want %q", tracks[0].Artwork.CacheKey, "feed:"+imageURL)
	}
	if tracks[1].Artwork.CacheKey != tracks[0].Artwork.CacheKey {
		t.Fatalf("artwork cache keys differ: %q vs %q", tracks[0].Artwork.CacheKey, tracks[1].Artwork.CacheKey)
	}
}

func TestResolveFeedResolvesRelativeArtworkURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Podcast</title>
    <itunes:image href="show.png" />
    <item>
      <title>Episode 1</title>
      <itunes:image href="episodes/ep1.png" />
      <enclosure url="https://cdn.example.com/ep1.mp3" type="audio/mpeg" />
    </item>
    <item>
      <title>Episode 2</title>
      <enclosure url="https://cdn.example.com/ep2.mp3" type="audio/mpeg" />
    </item>
  </channel>
</rss>`))
	}))
	defer server.Close()

	tracks, err := resolveFeed(server.URL + "/feeds/show.xml")
	if err != nil {
		t.Fatalf("resolveFeed() error = %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("resolveFeed() returned %d tracks, want 2", len(tracks))
	}

	wantItemURL := server.URL + "/feeds/episodes/ep1.png"
	if tracks[0].Artwork.URL != wantItemURL {
		t.Fatalf("episode artwork URL = %q, want %q", tracks[0].Artwork.URL, wantItemURL)
	}
	if tracks[0].Artwork.CacheKey != "feed:"+wantItemURL {
		t.Fatalf("episode artwork cache key = %q, want %q", tracks[0].Artwork.CacheKey, "feed:"+wantItemURL)
	}

	wantChannelURL := server.URL + "/feeds/show.png"
	if tracks[1].Artwork.URL != wantChannelURL {
		t.Fatalf("channel artwork URL = %q, want %q", tracks[1].Artwork.URL, wantChannelURL)
	}
	if tracks[1].Artwork.CacheKey != "feed:"+wantChannelURL {
		t.Fatalf("channel artwork cache key = %q, want %q", tracks[1].Artwork.CacheKey, "feed:"+wantChannelURL)
	}
}

func TestResolveFeedResolvesArtworkURLsAgainstRedirectedFeedURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			http.Redirect(w, r, "/feeds/show.xml", http.StatusFound)
		case "/feeds/show.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Podcast</title>
    <itunes:image href="show.png" />
    <item>
      <title>Episode 1</title>
      <enclosure url="https://cdn.example.com/ep1.mp3" type="audio/mpeg" />
    </item>
  </channel>
</rss>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tracks, err := resolveFeed(server.URL + "/feed")
	if err != nil {
		t.Fatalf("resolveFeed() error = %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("resolveFeed() returned %d tracks, want 1", len(tracks))
	}

	want := server.URL + "/feeds/show.png"
	if tracks[0].Artwork.URL != want {
		t.Fatalf("redirected artwork URL = %q, want %q", tracks[0].Artwork.URL, want)
	}
	if tracks[0].Artwork.CacheKey != "feed:"+want {
		t.Fatalf("redirected artwork cache key = %q, want %q", tracks[0].Artwork.CacheKey, "feed:"+want)
	}
}

func TestParseItunesDuration(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		// Plain seconds
		{"3600", 3600},
		{"90", 90},
		{"0", 0},
		// Fractional seconds
		{"3661.5", 3661},
		{"90.9", 90},
		// MM:SS
		{"1:30", 90},
		{"87:05", 5225},
		// HH:MM:SS
		{"1:27:05", 5225},
		{"0:01:30", 90},
		// Whitespace
		{" 3600 ", 3600},
		// Empty
		{"", 0},
		// Invalid — return 0
		{"abc", 0},
		{"12:xx", 0},
		{"1:2:xx", 0},
		// Negative — clamp to 0
		{"-1", 0},
		{"0:-10", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseItunesDuration(tt.input)
			if got != tt.want {
				t.Errorf("parseItunesDuration(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

type rewriteHostTransport struct {
	target *url.URL
	rt     http.RoundTripper
}

func (t rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return t.rt.RoundTrip(clone)
}
