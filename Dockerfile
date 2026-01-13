# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY *.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o instastream .

# Runtime stage - use Amazon Linux 2 for Lambda compatibility
FROM public.ecr.aws/lambda/python:3.11

# Install dependencies including tar and xz for extracting ffmpeg
RUN yum install -y tar xz && \
    pip install --no-cache-dir yt-dlp

# Install ffmpeg from static build
RUN curl -L https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz -o /tmp/ffmpeg.tar.xz && \
    tar -xf /tmp/ffmpeg.tar.xz -C /tmp && \
    cp /tmp/ffmpeg-*-amd64-static/ffmpeg /usr/local/bin/ && \
    cp /tmp/ffmpeg-*-amd64-static/ffprobe /usr/local/bin/ && \
    rm -rf /tmp/ffmpeg* && \
    yum clean all

# Copy the Go binary from builder
COPY --from=builder /app/instastream ${LAMBDA_TASK_ROOT}/

# Copy static files
COPY static/ ${LAMBDA_TASK_ROOT}/static/

# Set the entrypoint to our Go binary
ENTRYPOINT [ "./instastream" ]
