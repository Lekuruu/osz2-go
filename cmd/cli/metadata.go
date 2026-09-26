package main

import (
	"fmt"
	"io/fs"
	"time"

	"github.com/Lekuruu/osz2-go"
)

// Metadata represents the JSON structure for metadata output.
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

// FileMetadata represents file information in JSON format.
type FileMetadata struct {
	FileName     string    `json:"file_name"`
	Size         int64     `json:"size"`
	Hash         string    `json:"hash"`
	DateCreated  time.Time `json:"date_created"`
	DateModified time.Time `json:"date_modified"`
	BeatmapID    *int32    `json:"beatmap_id,omitempty"`
}

// HashData represents hash information.
type HashData struct {
	MetaDataHash string `json:"metadata_hash"`
	FileInfoHash string `json:"file_info_hash"`
	FullBodyHash string `json:"full_body_hash"`
}

func buildMetadata(reader *osz2.Reader) (*Metadata, error) {
	packageInfo := reader.Info()
	metadata := &Metadata{
		Attributes: make(map[string]string),
		Files:      make([]FileMetadata, 0),
		Hashes: HashData{
			MetaDataHash: fmt.Sprintf("%x", packageInfo.MetadataHash),
			FileInfoHash: fmt.Sprintf("%x", packageInfo.FileInfoHash),
			FullBodyHash: fmt.Sprintf("%x", packageInfo.BodyHash),
		},
	}

	for metaType, value := range reader.Metadata() {
		key := metaType.String()
		metadata.Attributes[key] = value

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

	err := fs.WalkDir(reader, ".", func(filename string, directoryEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if directoryEntry.IsDir() {
			return nil
		}

		entry, err := reader.Entry(filename)
		if err != nil {
			return err
		}

		fileMetadata := FileMetadata{
			FileName:     filename,
			Size:         entry.Size(),
			DateCreated:  entry.CreatedAt(),
			DateModified: entry.ModTime(),
			Hash:         fmt.Sprintf("%x", entry.Hash()),
		}
		if beatmapID, ok := entry.BeatmapID(); ok {
			fileMetadata.BeatmapID = &beatmapID
		}
		metadata.Files = append(metadata.Files, fileMetadata)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return metadata, nil
}
