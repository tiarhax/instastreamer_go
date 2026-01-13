# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY *.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o instastream .

# Runtime stage
FROM python:3.11-slim

WORKDIR /app

# Install yt-dlp and ffmpeg (for some video formats)
RUN pip install --no-cache-dir yt-dlp && \
    apt-get update && \
    apt-get install -y --no-install-recommends ffmpeg && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy the Go binary from builder
COPY --from=builder /app/instastream .

# Copy static files
COPY static/ ./static/

# Expose port
EXPOSE 8080

# Run the server
CMD ["./instastream"]
