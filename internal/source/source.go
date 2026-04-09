package source

import "cliamp/playlist"

type Kind string

const (
	Playlist Kind = "provider_playlist"
	Album    Kind = "album"
)

type Ref struct {
	ProviderKey string `json:"provider_key,omitempty"`
	Kind        Kind   `json:"kind,omitempty"`
	ID          string `json:"id,omitempty"`
}

func (r Ref) Valid() bool {
	return r.ProviderKey != "" && r.Kind != "" && r.ID != ""
}

type Restorer interface {
	RestoreSource(source Ref) ([]playlist.Track, error)
}

type Matcher interface {
	ResumeMetaKey() string
}
