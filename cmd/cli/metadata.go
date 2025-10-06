package main

import (
	"fmt"
	"time"

	"github.com/Lekuruu/osz2-go"
)

// Metadata represents the JSON structure for metadata output
type Metadata struct {
	Title         string            `json:"title,omitempty"`
	Artist        string            `json:"artist,omitempty"`
	Creator       string            `json:"creator,omitempty"`
	Version       string            `json:"version,omitempty"`
	Source        string            `json:"source,omitempty"`
	Tags          string            `json:"tags,omitempty"`
	BeatmapSetID  string            `json:"beatmap_set_id,omitempty"`
	Genre         string            `json:"genre,omitempty"`
	Language      string            `json:"language,omitempty"`
	TitleUnicode  string            `json:"title_unicode,omitempty"`
	ArtistUnicode string            `json:"artist_unicode,omitempty"`
	Difficulty    string            `json:"difficulty,omitempty"`
	PreviewTime   string            `json:"preview_time,omitempty"`
	Attributes    map[string]string `json:"attributes"`
	Files         []FileMetadata    `json:"files"`
	Hashes        HashData          `json:"hashes"`
}

// FileMetadata represents file information in JSON format
type FileMetadata struct {
	FileName     string    `json:"file_name"`
	Size         int32     `json:"size"`
	Hash         string    `json:"hash"`
	DateCreated  time.Time `json:"date_created"`
	DateModified time.Time `json:"date_modified"`
	BeatmapID    int32     `json:"beatmap_id,omitempty"`
}

// HashData represents hash information
type HashData struct {
	MetaDataHash string `json:"metadata_hash"`
	FileInfoHash string `json:"file_info_hash"`
	FullBodyHash string `json:"full_body_hash"`
}

func buildMetadata(pkg *osz2.Package) *Metadata {
	metadata := &Metadata{
		Attributes: make(map[string]string),
		Files:      make([]FileMetadata, 0),
		Hashes: HashData{
			MetaDataHash: fmt.Sprintf("%x", pkg.MetaDataHash),
			FileInfoHash: fmt.Sprintf("%x", pkg.FileInfoHash),
			FullBodyHash: fmt.Sprintf("%x", pkg.FullBodyHash),
		},
	}

	// Convert all metadata to string map
	for metaType, value := range pkg.Metadata {
		key := metaType.String()
		metadata.Attributes[key] = value

		// Also populate specific fields
		switch metaType {
		case osz2.Title:
			metadata.Title = value
		case osz2.Artist:
			metadata.Artist = value
		case osz2.Creator:
			metadata.Creator = value
		case osz2.Version:
			metadata.Version = value
		case osz2.Source:
			metadata.Source = value
		case osz2.Tags:
			metadata.Tags = value
		case osz2.BeatmapSetID:
			metadata.BeatmapSetID = value
		case osz2.Genre:
			metadata.Genre = value
		case osz2.Language:
			metadata.Language = value
		case osz2.TitleUnicode:
			metadata.TitleUnicode = value
		case osz2.ArtistUnicode:
			metadata.ArtistUnicode = value
		case osz2.Difficulty:
			metadata.Difficulty = value
		case osz2.PreviewTime:
			metadata.PreviewTime = value
		}
	}

	// Add file information
	for fileName, fileInfo := range pkg.FileInfos {
		fileMeta := FileMetadata{
			FileName:     fileName,
			Size:         fileInfo.Size,
			DateCreated:  fileInfo.DateCreated,
			DateModified: fileInfo.DateModified,
			Hash:         fmt.Sprintf("%x", fileInfo.Hash),
		}

		// Add beatmap ID if available
		if beatmapID, ok := pkg.FileNames[fileName]; ok {
			fileMeta.BeatmapID = beatmapID
		}

		metadata.Files = append(metadata.Files, fileMeta)
	}

	return metadata
}
