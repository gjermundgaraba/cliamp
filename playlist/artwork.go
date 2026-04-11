package playlist

type ArtworkKind uint8

const (
	ArtworkKindNone ArtworkKind = iota
	ArtworkKindEmbeddedFile
	ArtworkKindRemoteURL
)

type ArtworkRef struct {
	Kind     ArtworkKind
	CacheKey string
	Path     string
	URL      string
}

func NoArtwork() ArtworkRef { return ArtworkRef{} }

func EmbeddedArtwork(path string) ArtworkRef {
	if path == "" {
		return NoArtwork()
	}
	return ArtworkRef{Kind: ArtworkKindEmbeddedFile, Path: path}
}

func RemoteArtwork(cacheKey, rawURL string) ArtworkRef {
	if cacheKey == "" || rawURL == "" {
		return NoArtwork()
	}
	return ArtworkRef{Kind: ArtworkKindRemoteURL, CacheKey: cacheKey, URL: rawURL}
}

func (r ArtworkRef) IsNone() bool { return r.Kind == ArtworkKindNone }

func DefaultArtworkForPath(path string) ArtworkRef {
	if path == "" {
		return NoArtwork()
	}
	if IsLocalFilePath(path) {
		return EmbeddedArtwork(path)
	}
	return InferArtworkFromPath(path)
}
