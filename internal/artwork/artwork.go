package artwork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cliamp/internal/appdir"
	"cliamp/playlist"

	"github.com/dhowden/tag"
	"golang.org/x/sync/singleflight"
)

type Materializer struct {
	httpClient *http.Client
	group      singleflight.Group
	pruneOnce  sync.Once
}

type TrackRefResolver func(context.Context, playlist.Track) (playlist.ArtworkRef, error)

type ResolvedTrackArtwork struct {
	Ref  playlist.ArtworkRef
	Path string
}

const maxArtworkBytes = 12 << 20

var errUnsupportedImage = errors.New("artwork: unsupported image data")

func NewMaterializer() *Materializer {
	return &Materializer{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func ResolveTrack(ctx context.Context, materializer *Materializer, track playlist.Track, resolveRef TrackRefResolver) (ResolvedTrackArtwork, error) {
	if materializer == nil {
		return ResolvedTrackArtwork{}, nil
	}

	ref := track.Artwork
	if ref.IsNone() && resolveRef != nil {
		var err error
		ref, err = resolveRef(ctx, track)
		if err != nil {
			return ResolvedTrackArtwork{}, err
		}
	}
	if ref.IsNone() {
		return ResolvedTrackArtwork{}, nil
	}

	path, err := materializer.Materialize(ctx, ref)
	if err != nil {
		return ResolvedTrackArtwork{Ref: ref}, err
	}
	return ResolvedTrackArtwork{Ref: ref, Path: path}, nil
}

func (m *Materializer) Materialize(ctx context.Context, ref playlist.ArtworkRef) (string, error) {
	if ref.IsNone() {
		return "", nil
	}

	cacheKey, err := cacheKeyFor(ref)
	if err != nil {
		return "", err
	}

	cacheDir, err := artworkCacheDir()
	if err != nil {
		return "", err
	}
	m.pruneOnce.Do(func() { pruneArtworkCache(cacheDir, 30*24*time.Hour) })

	value, err, _ := m.group.Do(cacheKey, func() (any, error) {
		entryDir := filepath.Join(cacheDir, cacheKey)
		if cached, ok := existingCachedResult(entryDir); ok {
			return cached, nil
		}

		data, ext, err := m.fetch(ctx, ref)
		if err != nil {
			return "", err
		}
		if len(data) == 0 || ext == "" {
			if err := writeMissMarker(entryDir); err != nil {
				return "", err
			}
			return "", nil
		}
		path, err := writeCachedFile(entryDir, ext, data)
		if err != nil {
			return "", err
		}
		return path, nil
	})
	if err != nil {
		return "", err
	}
	if path, ok := value.(string); ok {
		return path, nil
	}
	return "", nil
}

func cacheKeyFor(ref playlist.ArtworkRef) (string, error) {
	switch ref.Kind {
	case playlist.ArtworkKindEmbeddedFile:
		abs, err := filepath.Abs(ref.Path)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return "", err
		}
		return stableKey(fmt.Sprintf("embedded:%s:%d:%d", abs, info.Size(), info.ModTime().UnixNano())), nil
	case playlist.ArtworkKindRemoteURL:
		return stableKey("remote:" + ref.CacheKey), nil
	default:
		return "", nil
	}
}

func stableKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func artworkCacheDir() (string, error) {
	dir, err := appdir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache", "artwork"), nil
}

func existingCachedResult(entryDir string) (string, bool) {
	info, err := os.Stat(entryDir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(entryDir, "missing")); err == nil {
		return "", true
	}
	matches, err := filepath.Glob(filepath.Join(entryDir, "image.*"))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return matches[0], true
}

func writeCachedFile(entryDir, ext string, data []byte) (string, error) {
	if err := os.MkdirAll(entryDir, 0o700); err != nil {
		return "", err
	}
	finalPath := filepath.Join(entryDir, "image."+ext)
	tmp, err := os.CreateTemp(entryDir, "tmp-*."+ext)
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", err
	}
	return finalPath, nil
}

func writeMissMarker(entryDir string) error {
	if err := os.MkdirAll(entryDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(entryDir, "missing"), nil, 0o600)
}

func (m *Materializer) fetch(ctx context.Context, ref playlist.ArtworkRef) ([]byte, string, error) {
	switch ref.Kind {
	case playlist.ArtworkKindEmbeddedFile:
		return extractEmbeddedArtwork(ref.Path)
	case playlist.ArtworkKindRemoteURL:
		return m.download(ctx, ref.URL)
	default:
		return nil, "", nil
	}
}

func extractEmbeddedArtwork(path string) ([]byte, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	meta, err := tag.ReadFrom(f)
	if err != nil || meta == nil {
		return nil, "", nil
	}
	pic := meta.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil, "", nil
	}
	data, ext, err := normalizeImage(pic.Data)
	if errors.Is(err, errUnsupportedImage) {
		return nil, "", nil
	}
	return data, ext, err
}

func (m *Materializer) download(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "cliamp/1.0 (artwork)")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("artwork: http status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArtworkBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxArtworkBytes {
		return nil, "", fmt.Errorf("artwork: image too large")
	}
	return normalizeImage(data)
}

func normalizeImage(data []byte) ([]byte, string, error) {
	sniffedExt := sniffImageExt(data)
	if sniffedExt == "" {
		return nil, "", errUnsupportedImage
	}
	return data, sniffedExt, nil
}

func sniffImageExt(data []byte) string {
	ct := http.DetectContentType(data)
	switch {
	case strings.HasPrefix(ct, "image/jpeg"):
		return "jpg"
	case strings.HasPrefix(ct, "image/png"):
		return "png"
	case strings.HasPrefix(ct, "image/webp"):
		return "webp"
	default:
		return ""
	}
}

func pruneArtworkCache(root string, maxAge time.Duration) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(path)
	}
}
