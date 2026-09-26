package osz2

import (
	"path"
	"strings"
)

const (
	beatmapExtension         = "osu"
	combinedBeatmapExtension = "osc"
)

// osu! officially only uses ".avi", ".flv" and ".mpg" for
// video files, so lets hope this won't cause any issues
var videoFileExtensions = map[string]struct{}{
	"wmv": {}, "flv": {}, "avi": {}, "m4v": {}, "mpg": {}, "mov": {},
	"webm": {}, "ogv": {}, "mpeg": {}, "3gp": {}, "mkv": {}, "mp4": {},
}

func extensionOf(name string) string {
	extension := strings.ToLower(path.Ext(strings.TrimSpace(name)))
	return strings.TrimPrefix(extension, ".")
}

func isBeatmapFileExtension(extension string) bool {
	return extension == beatmapExtension || extension == combinedBeatmapExtension
}

func isVideoExtension(extension string) bool {
	_, ok := videoFileExtensions[extension]
	return ok
}
