package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	AppPort             string
	JWTSecret           string
	SQLitePath          string
	MediaRoot           string
	MaxUploadSizeBytes  int64
	InitialUserEmail    string
	InitialUserPassword string
	AllowedOrigins      string
}

func Load() Config {
	loadDotEnv(".env")

	cfg := Config{
		AppPort:             getEnv("APP_PORT", "8080"),
		JWTSecret:           os.Getenv("JWT_SECRET"),
		SQLitePath:          getEnv("SQLITE_PATH", "data/media.db"),
		MediaRoot:           getEnv("MEDIA_ROOT", "media"),
		InitialUserEmail:    getEnv("INITIAL_USER_EMAIL", "kalemmalek123@gmail.com"),
		InitialUserPassword: os.Getenv("INITIAL_USER_PASSWORD"),
		AllowedOrigins:      getEnv("ALLOWED_ORIGINS", "*"),
	}

	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters")
	}
	if cfg.InitialUserPassword == "" {
		log.Fatal("INITIAL_USER_PASSWORD is required")
	}

	maxMB := getEnvInt("MAX_UPLOAD_SIZE_MB", 512)
	if maxMB < 1 {
		log.Fatal("MAX_UPLOAD_SIZE_MB must be at least 1")
	}
	cfg.MaxUploadSizeBytes = int64(maxMB) * 1024 * 1024

	return cfg
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("%s must be an integer", key)
	}
	return parsed
}

func loadDotEnv(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	lines := splitLines(string(content))
	for _, line := range lines {
		key, value, ok := parseEnvLine(line)
		if !ok {
			continue
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start <= len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func parseEnvLine(line string) (string, string, bool) {
	line = trimSpace(line)
	if line == "" || line[0] == '#' {
		return "", "", false
	}
	for i := 0; i < len(line); i++ {
		if line[i] == '=' {
			key := trimSpace(line[:i])
			value := trimSpace(line[i+1:])
			if key == "" {
				return "", "", false
			}
			if len(value) >= 2 {
				if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
					value = value[1 : len(value)-1]
				}
			}
			return key, value, true
		}
	}
	return "", "", false
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
