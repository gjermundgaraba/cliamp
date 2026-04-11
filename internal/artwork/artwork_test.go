package artwork

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cliamp/playlist"
)

func TestMaterializerReusesCachedFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(tinyPNG())
	}))
	defer server.Close()

	ref := playlist.RemoteArtwork("test:cover", server.URL+"/cover.png")
	materializer := NewMaterializer()

	first, err := materializer.Materialize(context.Background(), ref)
	if err != nil {
		t.Fatalf("first Materialize() error = %v", err)
	}
	second, err := materializer.Materialize(context.Background(), ref)
	if err != nil {
		t.Fatalf("second Materialize() error = %v", err)
	}
	if first == "" || second == "" {
		t.Fatalf("Materialize() returned empty paths: %q %q", first, second)
	}
	if first != second {
		t.Fatalf("Materialize() paths differ: %q vs %q", first, second)
	}
	if hits != 1 {
		t.Fatalf("server hit count = %d, want 1", hits)
	}
}

func TestMaterializerRejectsOversizedDownloads(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(strings.Repeat("a", maxArtworkBytes+1)))
	}))
	defer server.Close()

	ref := playlist.RemoteArtwork("overflow", server.URL+"/cover.png")
	materializer := NewMaterializer()
	_, err := materializer.Materialize(context.Background(), ref)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("Materialize() error = %v, want oversized image error", err)
	}

	cacheKey, err := cacheKeyFor(ref)
	if err != nil {
		t.Fatalf("cacheKeyFor() error = %v", err)
	}
	cacheDir, err := artworkCacheDir()
	if err != nil {
		t.Fatalf("artworkCacheDir() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, cacheKey)); !os.IsNotExist(err) {
		t.Fatalf("oversized artwork cache entry exists, stat err = %v", err)
	}
}

func TestMaterializerCachesUnknownFormatFromContentType(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(tinyPNG())
	}))
	defer server.Close()

	ref := playlist.RemoteArtwork("sniff", server.URL+"/cover")
	materializer := NewMaterializer()
	path, err := materializer.Materialize(context.Background(), ref)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("Materialize() path = %q, want .png suffix", path)
	}
}

func TestMaterializerUsesSniffedExtensionOverURLPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(tinyPNG())
	}))
	defer server.Close()

	ref := playlist.RemoteArtwork("mismatch", server.URL+"/cover.jpg")
	materializer := NewMaterializer()
	path, err := materializer.Materialize(context.Background(), ref)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("Materialize() path = %q, want .png suffix", path)
	}
}

func TestMaterializerRetriesInvalidRemoteResponses(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		path        string
	}{
		{
			name:        "non image content",
			contentType: "text/html",
			path:        "/cover.php",
		},
		{
			name:        "invalid image body with image hints",
			contentType: "image/png",
			path:        "/cover.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			hits := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.Header().Set("Content-Type", tt.contentType)
				_, _ = w.Write([]byte("<html></html>"))
			}))
			defer server.Close()

			ref := playlist.RemoteArtwork("bad-image", server.URL+tt.path)
			materializer := NewMaterializer()

			path, err := materializer.Materialize(context.Background(), ref)
			if err == nil {
				t.Fatal("first Materialize() error = nil, want unsupported image error")
			}
			if path != "" {
				t.Fatalf("first Materialize() path = %q, want empty", path)
			}

			path, err = materializer.Materialize(context.Background(), ref)
			if err == nil {
				t.Fatal("second Materialize() error = nil, want unsupported image error")
			}
			if path != "" {
				t.Fatalf("second Materialize() path = %q, want empty", path)
			}
			if hits != 2 {
				t.Fatalf("server hit count = %d, want 2", hits)
			}

			cacheKey, err := cacheKeyFor(ref)
			if err != nil {
				t.Fatalf("cacheKeyFor() error = %v", err)
			}
			cacheDir, err := artworkCacheDir()
			if err != nil {
				t.Fatalf("artworkCacheDir() error = %v", err)
			}
			if _, err := os.Stat(filepath.Join(cacheDir, cacheKey)); !os.IsNotExist(err) {
				t.Fatalf("invalid remote artwork cache entry exists, stat err = %v", err)
			}
		})
	}
}

func TestResolveTrackUsesExistingTrackArtworkBeforeResolver(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(tinyPNG())
	}))
	defer server.Close()

	track := playlist.Track{
		Artwork: playlist.RemoteArtwork("track:cover", server.URL+"/cover.png"),
	}
	resolverCalls := 0
	resolved, err := ResolveTrack(context.Background(), NewMaterializer(), track, func(context.Context, playlist.Track) (playlist.ArtworkRef, error) {
		resolverCalls++
		return playlist.RemoteArtwork("resolver:cover", server.URL+"/resolver.png"), nil
	})
	if err != nil {
		t.Fatalf("ResolveTrack() error = %v", err)
	}
	if resolverCalls != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolverCalls)
	}
	if resolved.Ref.CacheKey != "track:cover" {
		t.Fatalf("ResolveTrack() ref cache key = %q, want track:cover", resolved.Ref.CacheKey)
	}
	if resolved.Path == "" {
		t.Fatal("ResolveTrack() path is empty")
	}
}

func TestResolveTrackFallsBackToResolver(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(tinyPNG())
	}))
	defer server.Close()

	resolverCalls := 0
	resolved, err := ResolveTrack(context.Background(), NewMaterializer(), playlist.Track{}, func(context.Context, playlist.Track) (playlist.ArtworkRef, error) {
		resolverCalls++
		return playlist.RemoteArtwork("resolver:cover", server.URL+"/cover.png"), nil
	})
	if err != nil {
		t.Fatalf("ResolveTrack() error = %v", err)
	}
	if resolverCalls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolverCalls)
	}
	if resolved.Ref.CacheKey != "resolver:cover" {
		t.Fatalf("ResolveTrack() ref cache key = %q, want resolver:cover", resolved.Ref.CacheKey)
	}
	if resolved.Path == "" {
		t.Fatal("ResolveTrack() path is empty")
	}
}

func TestMaterializerCachesEmbeddedMisses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(path, []byte("not an audio file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	materializer := NewMaterializer()
	ref := playlist.EmbeddedArtwork(path)

	got, err := materializer.Materialize(context.Background(), ref)
	if err != nil {
		t.Fatalf("first Materialize() error = %v", err)
	}
	if got != "" {
		t.Fatalf("first Materialize() path = %q, want empty", got)
	}

	cacheKey, err := cacheKeyFor(ref)
	if err != nil {
		t.Fatalf("cacheKeyFor() error = %v", err)
	}
	cacheDir, err := artworkCacheDir()
	if err != nil {
		t.Fatalf("artworkCacheDir() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, cacheKey, "missing")); err != nil {
		t.Fatalf("missing artwork cache marker missing, stat err = %v", err)
	}

	got, err = materializer.Materialize(context.Background(), ref)
	if err != nil {
		t.Fatalf("second Materialize() error = %v", err)
	}
	if got != "" {
		t.Fatalf("second Materialize() path = %q, want empty", got)
	}
}

func TestSniffImageExt(t *testing.T) {
	if got := sniffImageExt(tinyPNG()); got != "png" {
		t.Fatalf("sniffImageExt(png) = %q, want png", got)
	}
	if got := sniffImageExt([]byte("not an image")); got != "" {
		t.Fatalf("sniffImageExt(text) = %q, want empty", got)
	}
}

func tinyPNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
