package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

type Service struct {
	db        *sql.DB
	jwtSecret []byte
}

func NewService(db *sql.DB, jwtSecret string) *Service {
	return &Service{db: db, jwtSecret: []byte(jwtSecret)}
}

func (s *Service) SeedInitialUser(email, password string) error {
	existing, err := s.UserByEmail(context.Background(), email)
	if err == nil && existing.ID > 0 {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO users (email, password_hash) VALUES (?, ?)`, strings.ToLower(email), string(hash))
	return err
}

func (s *Service) Authenticate(ctx context.Context, email, password string) (User, string, error) {
	user, hash, err := s.userWithHashByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return User{}, "", errors.New("invalid email or password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, "", errors.New("invalid email or password")
	}

	token, err := s.SignToken(user, 7*24*time.Hour)
	if err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

func (s *Service) UserByID(ctx context.Context, id int64) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `SELECT id, email FROM users WHERE id = ?`, id).Scan(&user.ID, &user.Email)
	return user, err
}

func (s *Service) UserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `SELECT id, email FROM users WHERE email = ?`, strings.ToLower(strings.TrimSpace(email))).Scan(&user.ID, &user.Email)
	return user, err
}

func (s *Service) userWithHashByEmail(ctx context.Context, email string) (User, string, error) {
	var user User
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT id, email, password_hash FROM users WHERE email = ?`, email).Scan(&user.ID, &user.Email, &hash)
	return user, hash, err
}

func (s *Service) SignToken(user User, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	claims := map[string]any{
		"sub":   strconv.FormatInt(user.ID, 10),
		"email": user.Email,
		"iat":   now.Unix(),
		"exp":   now.Add(ttl).Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := encodedHeader + "." + encodedClaims
	signature := s.sign(signingInput)

	return signingInput + "." + signature, nil
}

func (s *Service) VerifyToken(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, errors.New("invalid token")
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := s.sign(signingInput)
	if !hmac.Equal([]byte(expectedSig), []byte(parts[2])) {
		return 0, errors.New("invalid token signature")
	}

	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, errors.New("invalid token header")
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return 0, errors.New("invalid token header")
	}
	if header.Alg != "HS256" || header.Typ != "JWT" {
		return 0, errors.New("unsupported token")
	}

	var claims struct {
		Subject string `json:"sub"`
		Exp     int64  `json:"exp"`
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, errors.New("invalid token claims")
	}
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return 0, errors.New("invalid token claims")
	}
	if claims.Exp <= time.Now().UTC().Unix() {
		return 0, errors.New("token expired")
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return 0, errors.New("invalid token subject")
	}
	return userID, nil
}

func (s *Service) sign(input string) string {
	mac := hmac.New(sha256.New, s.jwtSecret)
	_, _ = mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func ExtractBearer(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", fmt.Errorf("authorization header must be Bearer token")
	}
	return parts[1], nil
}
