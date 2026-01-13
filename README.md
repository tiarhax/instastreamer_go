# Instagram Video Streamer (Go)

A Go-based web server that streams Instagram videos directly to the browser without storing them on the server.

## Prerequisites

- Go 1.21+
- `yt-dlp` installed and available in PATH

## Installation

```bash
# Install yt-dlp if not already installed
pip install yt-dlp

# Or on Ubuntu/Debian
sudo apt install yt-dlp
```

## Environment Variables

Copy `.env.example` to `.env` and configure:

```bash
cp .env.example .env
```

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AWS_REGION` | Yes (for auth) | - | AWS region for DynamoDB |
| `AWS_ACCESS_KEY_ID` | Yes (for auth) | - | AWS access key |
| `AWS_SECRET_ACCESS_KEY` | Yes (for auth) | - | AWS secret key |
| `DYNAMODB_TABLE` | No | `instastream-auth-codes` | DynamoDB table name |

## Running Locally

### Without Auth (Development Mode)

If AWS credentials are not configured, auth is bypassed:

```bash
go run main.go
```

### With Auth

```bash
# Set environment variables
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=your_key
export AWS_SECRET_ACCESS_KEY=your_secret
export DYNAMODB_TABLE=instastream-auth-codes

go run main.go
```

Then open http://localhost:8080 in your browser.

## DynamoDB Table Schema

| Attribute | Type | Description |
|-----------|------|-------------|
| `code` (PK) | String | Access code, format: `XXX-XXX` |
| `name` | String | User's display name |

## Docker

```bash
# Build
docker build -t instastream .

# Run with auth
docker run -p 8080:8080 \
  -e AWS_REGION=us-east-1 \
  -e AWS_ACCESS_KEY_ID=your_key \
  -e AWS_SECRET_ACCESS_KEY=your_secret \
  -e DYNAMODB_TABLE=instastream-auth-codes \
  instastream
```

## How it works

1. User enters access code (validated against DynamoDB)
2. User pastes an Instagram URL in the browser
3. Backend uses `yt-dlp` to extract the direct video URL
4. Backend streams the video directly to the browser using `yt-dlp -o -` (output to stdout)
5. Video is displayed in the browser and can be downloaded as a blob

**No server-side storage is used** — the video data flows directly from Instagram → yt-dlp → HTTP response → browser.

## API Endpoints

- `GET /` — Serves the web UI
- `POST /api/auth` — Validates access code against DynamoDB
- `POST /api/info` — Gets video metadata (requires `X-Auth-Code` header)
- `GET /api/stream?url=<url>&auth=<code>` — Streams the video
# instastreamer_go
