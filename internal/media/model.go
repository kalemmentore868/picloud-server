package media

import "time"

const (
	TypeMusic = "music"
	TypeVideo = "video"
	TypePhoto = "photo"
)

type Item struct {
	ID                int64     `json:"id"`
	OwnerUserID       int64     `json:"owner_user_id"`
	OriginalFilename  string    `json:"original_filename"`
	StoredFilename    string    `json:"stored_filename"`
	RelativePath      string    `json:"relative_path"`
	MediaType         string    `json:"media_type"`
	MimeType          string    `json:"mime_type"`
	FileSize          int64     `json:"file_size"`
	Title             *string   `json:"title,omitempty"`
	Artist            *string   `json:"artist,omitempty"`
	Album             *string   `json:"album,omitempty"`
	DurationSeconds   *int64    `json:"duration_seconds,omitempty"`
	Width             *int64    `json:"width,omitempty"`
	Height            *int64    `json:"height,omitempty"`
	ThumbnailPath     *string   `json:"thumbnail_path,omitempty"`
	ThumbnailMimeType *string   `json:"thumbnail_mime_type,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type CreateInput struct {
	OwnerUserID      int64
	OriginalFilename string
	StoredFilename   string
	RelativePath     string
	MediaType        string
	MimeType         string
	FileSize         int64
	Title            *string
	Artist           *string
	Album            *string
	Width            *int64
	Height           *int64
}

type UpdateInput struct {
	Title           *string
	Artist          *string
	Album           *string
	DurationSeconds *int64
	Width           *int64
	Height          *int64
}

type ThumbnailInput struct {
	ThumbnailPath     *string
	ThumbnailMimeType *string
}

type ListOptions struct {
	OwnerUserID int64
	MediaType   string
	Query       string
	Limit       int
	Offset      int
}

type ListResult struct {
	Items  []Item `json:"items"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	Total  int    `json:"total"`
}
