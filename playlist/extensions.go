package playlist

import (
	"path/filepath"
	"strings"
)

var SupportedAudioExts = map[string]bool{
	".mp3":  true,
	".wav":  true,
	".flac": true,
	".ogg":  true,
	".m4a":  true,
	".aac":  true,
	".m4b":  true,
	".alac": true,
	".wma":  true,
	".opus": true,
	".webm": true,
}

func HasSupportedAudioExt(path string) bool {
	return SupportedAudioExts[strings.ToLower(filepath.Ext(path))]
}
