package media

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

func (s *Store) EnsureDirs() error {
	for _, dir := range []string{TypeMusic, TypeVideo, TypePhoto, "thumbnails"} {
		if err := os.MkdirAll(filepath.Join(s.root, dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SaveThumbnail(reader io.Reader, originalFilename string) (relative string, fullPath string, mimeType string, err error) {
	ext := strings.ToLower(filepath.Ext(originalFilename))
	if ext == "" {
		ext = ".jpg"
	}
	mimeType = mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return "", "", "", fmt.Errorf("thumbnail must be an image")
	}

	stored := randomHex(16) + ext
	relative = filepath.ToSlash(filepath.Join("thumbnails", stored))
	fullPath, err = s.FullPath(relative)
	if err != nil {
		return "", "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", "", "", err
	}

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", "", "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		_ = os.Remove(fullPath)
		return "", "", "", err
	}
	return relative, fullPath, mimeType, nil
}

func (s *Store) Save(reader io.Reader, originalFilename, mediaType string) (stored string, relative string, fullPath string, err error) {
	ext := strings.ToLower(filepath.Ext(originalFilename))
	if ext == "" {
		ext = ".bin"
	}

	stored = randomHex(16) + ext
	relative = filepath.ToSlash(filepath.Join(mediaType, stored))
	fullPath, err = s.FullPath(relative)
	if err != nil {
		return "", "", "", err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", "", "", err
	}

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", "", "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		_ = os.Remove(fullPath)
		return "", "", "", err
	}
	return stored, relative, fullPath, nil
}

func (s *Store) FullPath(relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", errors.New("invalid relative path")
	}
	clean := filepath.Clean(relative)
	full := filepath.Join(s.root, clean)
	rootAbs, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+string(os.PathSeparator)) {
		return "", errors.New("path escapes media root")
	}
	return fullAbs, nil
}

func (s *Store) Delete(relative string) error {
	fullPath, err := s.FullPath(relative)
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func DetectType(filename string, sniff []byte) (string, string, error) {
	detected := http.DetectContentType(sniff)
	ext := strings.ToLower(filepath.Ext(filename))
	if byExt := mime.TypeByExtension(ext); byExt != "" && detected == "application/octet-stream" {
		detected = strings.Split(byExt, ";")[0]
	}

	if strings.HasPrefix(detected, "audio/") {
		return TypeMusic, detected, nil
	}
	if strings.HasPrefix(detected, "video/") {
		return TypeVideo, detected, nil
	}
	if strings.HasPrefix(detected, "image/") {
		return TypePhoto, detected, nil
	}

	switch ext {
	case ".mp3", ".m4a", ".aac", ".flac", ".wav", ".ogg", ".opus":
		return TypeMusic, mimeOrFallback(detected, "audio/mpeg"), nil
	case ".mp4", ".mov", ".m4v", ".webm", ".mkv":
		return TypeVideo, mimeOrFallback(detected, "video/mp4"), nil
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic":
		return TypePhoto, mimeOrFallback(detected, "image/jpeg"), nil
	default:
		return "", detected, fmt.Errorf("unsupported media type: %s", detected)
	}
}

func SafeOriginalFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	if name == "." || name == "" {
		return "upload"
	}
	return name
}

func randomHex(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func mimeOrFallback(detected, fallback string) string {
	if detected == "" || detected == "application/octet-stream" {
		return fallback
	}
	return detected
}
