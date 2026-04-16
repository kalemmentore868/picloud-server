package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"picloud-server/internal/auth"
	"picloud-server/internal/config"
	"picloud-server/internal/media"
	"picloud-server/internal/middleware"
)

type Server struct {
	cfg         config.Config
	authService *auth.Service
	repo        *media.Repository
	store       *media.Store
	loginLimit  *middleware.RateLimiter
}

func NewServer(cfg config.Config, authService *auth.Service, repo *media.Repository, store *media.Store) *Server {
	return &Server{
		cfg:         cfg,
		authService: authService,
		repo:        repo,
		store:       store,
		loginLimit:  middleware.NewRateLimiter(10, 10*time.Minute),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.Handle("POST /api/auth/login", s.loginLimit.Middleware(http.HandlerFunc(s.login)))
	mux.Handle("GET /api/auth/me", middleware.RequireAuth(s.authService, http.HandlerFunc(s.me)))
	mux.Handle("POST /api/media/upload", middleware.RequireAuth(s.authService, http.HandlerFunc(s.upload)))
	mux.Handle("GET /api/media", middleware.RequireAuth(s.authService, http.HandlerFunc(s.listMedia)))
	mux.Handle("GET /api/media/search", middleware.RequireAuth(s.authService, http.HandlerFunc(s.searchMedia)))
	mux.Handle("GET /api/media/{id}", middleware.RequireAuth(s.authService, http.HandlerFunc(s.getMedia)))
	mux.Handle("PATCH /api/media/{id}", middleware.RequireAuth(s.authService, http.HandlerFunc(s.updateMedia)))
	mux.Handle("POST /api/media/{id}/thumbnail", middleware.RequireAuth(s.authService, http.HandlerFunc(s.uploadThumbnail)))
	mux.Handle("GET /api/media/{id}/thumbnail", middleware.RequireAuth(s.authService, http.HandlerFunc(s.thumbnailMedia)))
	mux.Handle("GET /api/media/{id}/stream", middleware.RequireAuth(s.authService, http.HandlerFunc(s.streamMedia)))
	mux.Handle("GET /api/media/{id}/download", middleware.RequireAuth(s.authService, http.HandlerFunc(s.downloadMedia)))
	mux.Handle("DELETE /api/media/{id}", middleware.RequireAuth(s.authService, http.HandlerFunc(s.deleteMedia)))

	var handler http.Handler = mux
	handler = middleware.CORS(s.cfg.AllowedOrigins)(handler)
	handler = middleware.Logging(handler)
	handler = middleware.Recover(handler)
	return handler
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validEmail(req.Email) || req.Password == "" || len(req.Password) > 256 {
		writeError(w, http.StatusBadRequest, "invalid email or password")
		return
	}

	user, token, err := s.authService.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	user, err := s.authService.UserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadSizeBytes+1024*1024)
	if err := r.ParseMultipartForm(s.cfg.MaxUploadSizeBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart upload or file too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	originalName := media.SafeOriginalFilename(header.Filename)
	if header.Size <= 0 || header.Size > s.cfg.MaxUploadSizeBytes {
		writeError(w, http.StatusBadRequest, "file size is not allowed")
		return
	}

	buffer := make([]byte, 512)
	n, err := io.ReadFull(file, buffer)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "could not read upload")
		return
	}
	buffer = buffer[:n]

	mediaType, mimeType, err := media.DetectType(originalName, buffer)
	if err != nil {
		writeError(w, http.StatusUnsupportedMediaType, err.Error())
		return
	}

	reader := io.MultiReader(bytes.NewReader(buffer), io.LimitReader(file, s.cfg.MaxUploadSizeBytes-int64(n)+1))
	stored, relative, fullPath, err := s.store.Save(reader, originalName, mediaType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store file")
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		_ = s.store.Delete(relative)
		writeError(w, http.StatusInternalServerError, "could not inspect stored file")
		return
	}
	if info.Size() > s.cfg.MaxUploadSizeBytes {
		_ = s.store.Delete(relative)
		writeError(w, http.StatusBadRequest, "file too large")
		return
	}

	width, height := media.ImageDimensions(fullPath, mediaType)
	title := strings.TrimSpace(r.FormValue("title"))
	var titlePtr *string
	if title != "" {
		titlePtr = &title
	}

	item, err := s.repo.Create(r.Context(), media.CreateInput{
		OwnerUserID:      userID,
		OriginalFilename: originalName,
		StoredFilename:   stored,
		RelativePath:     relative,
		MediaType:        mediaType,
		MimeType:         mimeType,
		FileSize:         info.Size(),
		Title:            titlePtr,
		Width:            width,
		Height:           height,
	})
	if err != nil {
		_ = s.store.Delete(relative)
		writeError(w, http.StatusInternalServerError, "could not save metadata")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request) {
	opts, ok := s.listOptions(w, r)
	if !ok {
		return
	}
	result, err := s.repo.List(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list media")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) searchMedia(w http.ResponseWriter, r *http.Request) {
	opts, ok := s.listOptions(w, r)
	if !ok {
		return
	}
	if opts.Query == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	result, err := s.repo.Search(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not search media")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getMedia(w http.ResponseWriter, r *http.Request) {
	item, ok := s.itemFromRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateMedia(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req struct {
		Title           *string `json:"title"`
		Artist          *string `json:"artist"`
		Album           *string `json:"album"`
		DurationSeconds *int64  `json:"duration_seconds"`
		Width           *int64  `json:"width"`
		Height          *int64  `json:"height"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	trimStringPtr(req.Title)
	trimStringPtr(req.Artist)
	trimStringPtr(req.Album)
	if invalidPositivePtr(req.DurationSeconds) || invalidPositivePtr(req.Width) || invalidPositivePtr(req.Height) {
		writeError(w, http.StatusBadRequest, "numeric metadata must be positive")
		return
	}

	item, err := s.repo.Update(r.Context(), userID, id, media.UpdateInput{
		Title:           req.Title,
		Artist:          req.Artist,
		Album:           req.Album,
		DurationSeconds: req.DurationSeconds,
		Width:           req.Width,
		Height:          req.Height,
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update media")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) uploadThumbnail(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	existing, err := s.repo.Get(r.Context(), userID, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not get media")
		return
	}

	const maxThumbnailBytes = 8 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxThumbnailBytes+1024)
	if err := r.ParseMultipartForm(maxThumbnailBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid thumbnail upload or file too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	if header.Size <= 0 || header.Size > maxThumbnailBytes {
		writeError(w, http.StatusBadRequest, "thumbnail size is not allowed")
		return
	}

	buffer := make([]byte, 512)
	n, err := io.ReadFull(file, buffer)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "could not read thumbnail")
		return
	}
	buffer = buffer[:n]

	mimeType := http.DetectContentType(buffer)
	if !strings.HasPrefix(mimeType, "image/") {
		writeError(w, http.StatusUnsupportedMediaType, "thumbnail must be an image")
		return
	}

	reader := io.MultiReader(bytes.NewReader(buffer), io.LimitReader(file, maxThumbnailBytes-int64(n)+1))
	relative, _, storedMime, err := s.store.SaveThumbnail(reader, media.SafeOriginalFilename(header.Filename))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store thumbnail")
		return
	}
	if storedMime == "image/jpeg" && mimeType != "application/octet-stream" {
		storedMime = mimeType
	}

	item, err := s.repo.UpdateThumbnail(r.Context(), userID, id, media.ThumbnailInput{
		ThumbnailPath:     &relative,
		ThumbnailMimeType: &storedMime,
	})
	if err != nil {
		_ = s.store.Delete(relative)
		writeError(w, http.StatusInternalServerError, "could not save thumbnail metadata")
		return
	}

	if existing.ThumbnailPath != nil && *existing.ThumbnailPath != "" && *existing.ThumbnailPath != relative {
		_ = s.store.Delete(*existing.ThumbnailPath)
	}

	writeJSON(w, http.StatusOK, item)
}

func (s *Server) thumbnailMedia(w http.ResponseWriter, r *http.Request) {
	item, ok := s.itemFromRequest(w, r)
	if !ok {
		return
	}

	if item.ThumbnailPath != nil && *item.ThumbnailPath != "" {
		s.serveRelativeFile(w, r, *item.ThumbnailPath, item.ThumbnailMimeType, item.OriginalFilename+" thumbnail")
		return
	}
	if item.MediaType == media.TypePhoto {
		s.serveFile(w, r, item, false)
		return
	}

	writeError(w, http.StatusNotFound, "thumbnail not found")
}

func (s *Server) streamMedia(w http.ResponseWriter, r *http.Request) {
	item, ok := s.itemFromRequest(w, r)
	if !ok {
		return
	}
	s.serveFile(w, r, item, false)
}

func (s *Server) downloadMedia(w http.ResponseWriter, r *http.Request) {
	item, ok := s.itemFromRequest(w, r)
	if !ok {
		return
	}
	s.serveFile(w, r, item, true)
}

func (s *Server) deleteMedia(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	item, err := s.repo.Delete(r.Context(), userID, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete metadata")
		return
	}
	if err := s.store.Delete(item.RelativePath); err != nil {
		writeError(w, http.StatusInternalServerError, "metadata deleted but file removal failed")
		return
	}
	if item.ThumbnailPath != nil && *item.ThumbnailPath != "" {
		_ = s.store.Delete(*item.ThumbnailPath)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) itemFromRequest(w http.ResponseWriter, r *http.Request) (media.Item, bool) {
	userID, _ := middleware.UserID(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return media.Item{}, false
	}
	item, err := s.repo.Get(r.Context(), userID, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "media not found")
		return media.Item{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not get media")
		return media.Item{}, false
	}
	return item, true
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, item media.Item, download bool) {
	fullPath, err := s.store.FullPath(item.RelativePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid stored path")
		return
	}
	file, err := os.Open(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "file missing")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not open file")
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not inspect file")
		return
	}

	w.Header().Set("Content-Type", item.MimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Accept-Ranges", "bytes")
	if download {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(item.OriginalFilename)))
	}
	http.ServeContent(w, r, item.OriginalFilename, stat.ModTime(), file)
}

func (s *Server) serveRelativeFile(w http.ResponseWriter, r *http.Request, relative string, mimeType *string, filename string) {
	fullPath, err := s.store.FullPath(relative)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid stored path")
		return
	}
	file, err := os.Open(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "file missing")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not open file")
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not inspect file")
		return
	}

	contentType := "image/jpeg"
	if mimeType != nil && *mimeType != "" {
		contentType = *mimeType
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filename, stat.ModTime(), file)
}

func (s *Server) listOptions(w http.ResponseWriter, r *http.Request) (media.ListOptions, bool) {
	userID, _ := middleware.UserID(r.Context())
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	mediaType := strings.TrimSpace(r.URL.Query().Get("type"))
	if mediaType != "" && !validMediaType(mediaType) {
		writeError(w, http.StatusBadRequest, "invalid media type")
		return media.ListOptions{}, false
	}

	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)
	if limit < 1 || limit > 100 {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
		return media.ListOptions{}, false
	}
	if offset < 0 {
		writeError(w, http.StatusBadRequest, "offset must be 0 or greater")
		return media.ListOptions{}, false
	}

	return media.ListOptions{
		OwnerUserID: userID,
		MediaType:   mediaType,
		Query:       q,
		Limit:       limit,
		Offset:      offset,
	}, true
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid media id")
		return 0, false
	}
	return id, true
}

func decodeJSON(r *http.Request, dest any) error {
	if r.Header.Get("Content-Type") != "" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("content type must be application/json")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return errors.New("invalid json body")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("request body must contain one json object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func validEmail(email string) bool {
	email = strings.TrimSpace(email)
	return len(email) <= 254 && strings.Contains(email, "@") && strings.Contains(email, ".")
}

func validMediaType(value string) bool {
	return value == media.TypeMusic || value == media.TypeVideo || value == media.TypePhoto
}

func parseIntDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return parsed
}

func trimStringPtr(value *string) {
	if value != nil {
		*value = strings.TrimSpace(*value)
	}
}

func invalidPositivePtr(value *int64) bool {
	return value != nil && *value < 0
}
