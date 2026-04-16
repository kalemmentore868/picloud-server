# picloud-server

A lightweight self-hosted Go backend for a single-user personal media app. It stores media files on the local filesystem and keeps metadata in SQLite.

## Features

- Email/password login with bcrypt password hashing
- JWT auth for protected API routes
- SQLite auto-initialization and migrations on startup
- Local filesystem storage organized by media type
- Multipart uploads for music, videos, and photos
- MIME sniffing plus extension fallback
- Paginated listing, type filtering, and filename/title search
- Range-capable streaming through `http.ServeContent`
- Request logging, panic recovery, CORS, and login rate limiting
- Graceful shutdown
- Raspberry Pi friendly: one Go process, SQLite, and files on disk

## Project Structure

```text
cmd/server/              application entrypoint
internal/auth/           users, bcrypt, JWT signing and verification
internal/config/         environment and .env loading
internal/database/       SQLite open and startup migrations
internal/httpapi/        JSON API handlers and route wiring
internal/media/          media repository, filesystem store, type detection
internal/middleware/     auth, CORS, logging, recovery, rate limiting
migrations/              SQL migration reference
```

## Configuration

Copy the example env file and edit the secret:

```sh
cp .env.example .env
```

Generate a strong JWT secret:

```sh
openssl rand -base64 48
```

Required environment variables:

```text
APP_PORT=8080
JWT_SECRET=replace-with-at-least-32-random-characters
SQLITE_PATH=data/media.db
MEDIA_ROOT=media
MAX_UPLOAD_SIZE_MB=512
INITIAL_USER_EMAIL=kalemmalek123@gmail.com
INITIAL_USER_PASSWORD=TestPassword675
ALLOWED_ORIGINS=*
```

On startup the server loads `.env` if present, opens SQLite, runs migrations, creates media folders, seeds the initial user if missing, and starts the HTTP server.

## Run Locally

```sh
go mod download
go run ./cmd/server
```

Health check:

```sh
curl http://localhost:8080/health
```

## Run With Docker

```sh
cp .env.example .env
docker compose up -d --build
```

The compose file persists:

- SQLite database under `./data`
- media files under `./media`

## API

### Login

```sh
curl -s http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"kalemmalek123@gmail.com","password":"TestPassword675"}'
```

Save the returned token:

```sh
TOKEN="paste-token-here"
```

### Current User

```sh
curl http://localhost:8080/api/auth/me \
  -H "Authorization: Bearer $TOKEN"
```

### Upload

```sh
curl -X POST http://localhost:8080/api/media/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/path/to/song.mp3" \
  -F "title=My Song"
```

### List

```sh
curl "http://localhost:8080/api/media?limit=25&offset=0" \
  -H "Authorization: Bearer $TOKEN"
```

Filter by type:

```sh
curl "http://localhost:8080/api/media?type=music" \
  -H "Authorization: Bearer $TOKEN"
```

Search:

```sh
curl "http://localhost:8080/api/media/search?q=vacation" \
  -H "Authorization: Bearer $TOKEN"
```

### Get One Item

```sh
curl http://localhost:8080/api/media/1 \
  -H "Authorization: Bearer $TOKEN"
```

### Stream

```sh
curl -L http://localhost:8080/api/media/1/stream \
  -H "Authorization: Bearer $TOKEN" \
  -H "Range: bytes=0-"
```

### Download

```sh
curl -L -OJ http://localhost:8080/api/media/1/download \
  -H "Authorization: Bearer $TOKEN"
```

### Update Metadata

```sh
curl -X PATCH http://localhost:8080/api/media/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"New Title","artist":"Artist","album":"Album"}'
```

### Delete

```sh
curl -X DELETE http://localhost:8080/api/media/1 \
  -H "Authorization: Bearer $TOKEN"
```

## Endpoints

```text
GET    /health
POST   /api/auth/login
GET    /api/auth/me
POST   /api/media/upload
GET    /api/media
GET    /api/media/search?q=...
GET    /api/media/{id}
PATCH  /api/media/{id}
GET    /api/media/{id}/stream
GET    /api/media/{id}/download
DELETE /api/media/{id}
```

## Future iOS App Notes

- Store the JWT in the iOS Keychain.
- Attach `Authorization: Bearer <token>` to all media requests after login.
- Use `URLSessionUploadTask` with multipart form data for uploads.
- Use `AVPlayer` for audio/video playback pointed at `/api/media/{id}/stream`.
- Include the auth header when creating the playback request.
- Use `Range` requests automatically through `AVPlayer`; the backend supports them.
- Use `/api/media?type=photo`, `/api/media?type=music`, and `/api/media?type=video` to build separate tabs.

## Raspberry Pi Deployment Notes

This app is intentionally simple enough for a Raspberry Pi with 4 GB RAM:

- Keep `MEDIA_ROOT` on a disk or SD card path with enough free space.
- Back up both the SQLite file and the media directory together.
- Use Docker Compose or run the compiled Go binary as a systemd service.
- If using the SD card for media, avoid excessive write churn and keep backups.
- For large libraries, an external USB SSD is a better long-term media root.

## Cloudflare Tunnel Notes

Cloudflare Tunnel works well in front of this service:

1. Run the media server on localhost, for example `http://localhost:8080`.
2. Create a Cloudflare Tunnel that routes your hostname to that local URL.
3. Keep HTTPS termination at Cloudflare.
4. Set `ALLOWED_ORIGINS` to the future app/web origin if you add a browser frontend.
5. Keep `JWT_SECRET` private and rotate it if you suspect exposure.

For a mobile-only app, CORS is less important than it is for browsers, but leaving it configurable makes future web clients easier.

## Thumbnail And Metadata Extension Points

The upload path already records image dimensions for JPEG, PNG, and GIF photos. A lightweight future thumbnail endpoint can be added as:

```text
GET /api/media/{id}/thumbnail
```

Recommended future fields are already easy to add to SQLite, such as `thumbnail_relative_path` and `album_art_relative_path`. For audio duration and embedded tags, prefer adding a small metadata library later only when you need it.
