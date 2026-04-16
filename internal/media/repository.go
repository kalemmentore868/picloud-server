package media

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (Item, error) {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO media (
			owner_user_id, original_filename, stored_filename, relative_path,
			media_type, mime_type, file_size, title, artist, album, width, height
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.OwnerUserID, input.OriginalFilename, input.StoredFilename, input.RelativePath,
		input.MediaType, input.MimeType, input.FileSize, input.Title, input.Artist, input.Album, input.Width, input.Height,
	)
	if err != nil {
		return Item{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Item{}, err
	}
	return r.Get(ctx, input.OwnerUserID, id)
}

func (r *Repository) List(ctx context.Context, opts ListOptions) (ListResult, error) {
	where, args := buildListWhere(opts)
	countQuery := `SELECT COUNT(*) FROM media ` + where

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return ListResult{}, err
	}

	args = append(args, opts.Limit, opts.Offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, owner_user_id, original_filename, stored_filename, relative_path, media_type,
			mime_type, file_size, title, artist, album, duration_seconds, width, height,
			thumbnail_relative_path, thumbnail_mime_type, created_at, updated_at
		FROM media `+where+`
		ORDER BY datetime(created_at) DESC, id DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	items, err := scanItems(rows)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Limit: opts.Limit, Offset: opts.Offset, Total: total}, nil
}

func (r *Repository) Search(ctx context.Context, opts ListOptions) (ListResult, error) {
	return r.List(ctx, opts)
}

func (r *Repository) Get(ctx context.Context, ownerID, id int64) (Item, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, owner_user_id, original_filename, stored_filename, relative_path, media_type,
			mime_type, file_size, title, artist, album, duration_seconds, width, height,
			thumbnail_relative_path, thumbnail_mime_type, created_at, updated_at
		FROM media
		WHERE owner_user_id = ? AND id = ?`, ownerID, id)
	return scanItem(row)
}

func (r *Repository) UpdateThumbnail(ctx context.Context, ownerID, id int64, input ThumbnailInput) (Item, error) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE media SET
			thumbnail_relative_path = ?, thumbnail_mime_type = ?, updated_at = CURRENT_TIMESTAMP
		WHERE owner_user_id = ? AND id = ?`,
		input.ThumbnailPath, input.ThumbnailMimeType, ownerID, id,
	)
	if err != nil {
		return Item{}, err
	}
	return r.Get(ctx, ownerID, id)
}

func (r *Repository) Update(ctx context.Context, ownerID, id int64, input UpdateInput) (Item, error) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE media SET
			title = ?, artist = ?, album = ?, duration_seconds = ?, width = ?, height = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE owner_user_id = ? AND id = ?`,
		input.Title, input.Artist, input.Album, input.DurationSeconds, input.Width, input.Height, ownerID, id,
	)
	if err != nil {
		return Item{}, err
	}
	return r.Get(ctx, ownerID, id)
}

func (r *Repository) Delete(ctx context.Context, ownerID, id int64) (Item, error) {
	item, err := r.Get(ctx, ownerID, id)
	if err != nil {
		return Item{}, err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM media WHERE owner_user_id = ? AND id = ?`, ownerID, id)
	return item, err
}

func buildListWhere(opts ListOptions) (string, []any) {
	clauses := []string{"owner_user_id = ?"}
	args := []any{opts.OwnerUserID}
	if opts.MediaType != "" {
		clauses = append(clauses, "media_type = ?")
		args = append(args, opts.MediaType)
	}
	if opts.Query != "" {
		clauses = append(clauses, "(original_filename LIKE ? OR title LIKE ?)")
		like := "%" + opts.Query + "%"
		args = append(args, like, like)
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(row scanner) (Item, error) {
	var item Item
	var createdAt string
	var updatedAt string
	err := row.Scan(
		&item.ID, &item.OwnerUserID, &item.OriginalFilename, &item.StoredFilename, &item.RelativePath,
		&item.MediaType, &item.MimeType, &item.FileSize, &item.Title, &item.Artist, &item.Album,
		&item.DurationSeconds, &item.Width, &item.Height, &item.ThumbnailPath, &item.ThumbnailMimeType, &createdAt, &updatedAt,
	)
	if err != nil {
		return item, err
	}
	item.CreatedAt = parseSQLiteTime(createdAt)
	item.UpdatedAt = parseSQLiteTime(updatedAt)
	return item, err
}

func scanItems(rows *sql.Rows) ([]Item, error) {
	var items []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []Item{}
	}
	return items, nil
}

func parseSQLiteTime(value string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.ParseInLocation(layout, value, time.UTC)
		if err == nil {
			return parsed
		}
	}
	return time.Time{}
}
