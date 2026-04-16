FROM golang:1.23-bookworm AS build

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/picloud-server ./cmd/server

FROM debian:bookworm-slim

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates sqlite3 \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/picloud-server /usr/local/bin/picloud-server

EXPOSE 8080
CMD ["picloud-server"]
