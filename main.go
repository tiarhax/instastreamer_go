package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
)

type VideoInfo struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Extension string `json:"ext"`
}

type StreamRequest struct {
	URL string `json:"url"`
}

type AuthRequest struct {
	Code string `json:"code"`
}

type AuthResponse struct {
	Valid bool   `json:"valid"`
	Name  string `json:"name,omitempty"`
	Error string `json:"error,omitempty"`
}

var dynamoClient *dynamodb.Client
var tableName string
var httpAdapter *httpadapter.HandlerAdapterV2

func init() {
	// Initialize DynamoDB client
	initDynamoDB()

	// Create HTTP mux
	mux := http.NewServeMux()

	// Serve static files (HTML frontend)
	mux.HandleFunc("/", serveIndex)

	// Auth endpoint (no auth required)
	mux.HandleFunc("/api/auth", handleAuth)

	// API endpoint to get video info (auth required)
	mux.HandleFunc("/api/info", withAuth(handleVideoInfo))

	// API endpoint to stream video (auth required)
	mux.HandleFunc("/api/stream", withAuth(handleStream))

	// Create Lambda adapter for API Gateway HTTP API / Lambda Function URL
	httpAdapter = httpadapter.NewV2(mux)
}

func main() {
	// Check if running in Lambda environment
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		// Running in Lambda
		lambda.Start(httpAdapter.ProxyWithContext)
	} else {
		// Running locally
		log.Printf("Running in local mode")

		// Create HTTP mux for local server
		mux := http.NewServeMux()
		mux.HandleFunc("/", serveIndex)
		mux.HandleFunc("/api/auth", handleAuth)
		mux.HandleFunc("/api/info", withAuth(handleVideoInfo))
		mux.HandleFunc("/api/stream", withAuth(handleStream))

		port := ":8080"
		log.Printf("Server starting on http://localhost%s", port)
		log.Fatal(http.ListenAndServe(port, mux))
	}
}

func initDynamoDB() {
	tableName = os.Getenv("DYNAMODB_TABLE")
	if tableName == "" {
		tableName = "insta-stream-users"
	}

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Printf("Warning: Failed to load AWS config: %v", err)
		log.Printf("Auth will be disabled (local development mode)")
		return
	}

	dynamoClient = dynamodb.NewFromConfig(cfg)
	log.Printf("DynamoDB client initialized, table: %s", tableName)
}

func withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if DynamoDB is not configured (local dev)
		if dynamoClient == nil {
			next(w, r)
			return
		}

		// Get auth code from header
		authCode := r.Header.Get("X-Auth-Code")
		if authCode == "" {
			// Also check query param for stream endpoint
			authCode = r.URL.Query().Get("auth")
		}

		if authCode == "" {
			http.Error(w, "Unauthorized: missing auth code", http.StatusUnauthorized)
			return
		}

		// Validate code format
		if !isValidCodeFormat(authCode) {
			http.Error(w, "Unauthorized: invalid auth code format", http.StatusUnauthorized)
			return
		}

		// Validate against DynamoDB
		valid, _, err := validateCode(authCode)
		if err != nil {
			log.Printf("Auth validation error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if !valid {
			http.Error(w, "Unauthorized: invalid auth code", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func isValidCodeFormat(code string) bool {
	// Format: XXX-XXX where X is uppercase letter or digit
	pattern := `^[A-Z0-9]{3}-[A-Z0-9]{3}$`
	matched, _ := regexp.MatchString(pattern, strings.ToUpper(code))
	return matched
}

func validateCode(code string) (bool, string, error) {
	if dynamoClient == nil {
		return true, "Local User", nil
	}

	// Normalize code to uppercase
	code = strings.ToUpper(code)

	result, err := dynamoClient.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"code": &types.AttributeValueMemberS{Value: code},
		},
	})

	if err != nil {
		return false, "", err
	}

	if result.Item == nil {
		return false, "", nil
	}

	// Extract name from result
	name := ""
	if nameAttr, ok := result.Item["name"].(*types.AttributeValueMemberS); ok {
		name = nameAttr.Value
	}

	return true, name, nil
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{Valid: false, Error: "Invalid request body"})
		return
	}

	if req.Code == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{Valid: false, Error: "Code is required"})
		return
	}

	// Validate code format
	if !isValidCodeFormat(req.Code) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{Valid: false, Error: "Invalid code format. Expected: XXX-XXX"})
		return
	}

	valid, name, err := validateCode(req.Code)
	if err != nil {
		log.Printf("DynamoDB error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{Valid: false, Error: "Failed to validate code"})
		return
	}

	if !valid {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{Valid: false, Error: "Invalid code"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{Valid: true, Name: name})
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Read the HTML file
	content, err := os.ReadFile("static/index.html")
	if err != nil {
		log.Printf("Error reading index.html: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Explicitly set Content-Type for HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

func handleVideoInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Validate URL format
	parsedURL, err := url.Parse(req.URL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		http.Error(w, "URL must use http or https scheme", http.StatusBadRequest)
		return
	}
	if !strings.Contains(parsedURL.Host, "instagram.com") {
		http.Error(w, "URL must be an Instagram URL", http.StatusBadRequest)
		return
	}

	// Use yt-dlp to get video info (JSON output)
	cmd := exec.Command("yt-dlp", "-j", "--no-warnings", req.URL)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("yt-dlp error: %v", err)
		http.Error(w, "Failed to get video info", http.StatusInternalServerError)
		return
	}

	var info map[string]interface{}
	if err := json.Unmarshal(output, &info); err != nil {
		http.Error(w, "Failed to parse video info", http.StatusInternalServerError)
		return
	}

	// Extract the direct video URL
	videoURL := ""
	title := "video"
	ext := "mp4"

	if url, ok := info["url"].(string); ok {
		videoURL = url
	}
	if t, ok := info["title"].(string); ok {
		title = t
	}
	if e, ok := info["ext"].(string); ok {
		ext = e
	}

	response := VideoInfo{
		URL:       videoURL,
		Title:     title,
		Extension: ext,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	instagramURL := r.URL.Query().Get("url")
	if instagramURL == "" {
		http.Error(w, "URL parameter is required", http.StatusBadRequest)
		return
	}

	log.Printf("Streaming video from: %s", instagramURL)

	// Use yt-dlp to output video to stdout
	cmd := exec.Command("yt-dlp", "-o", "-", "--no-warnings", "-f", "best", instagramURL)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Failed to create stdout pipe: %v", err)
		http.Error(w, "Failed to start download", http.StatusInternalServerError)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("Failed to create stderr pipe: %v", err)
		http.Error(w, "Failed to start download", http.StatusInternalServerError)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start yt-dlp: %v", err)
		http.Error(w, "Failed to start download", http.StatusInternalServerError)
		return
	}

	// Log stderr in background
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.Contains(line, "[download]") {
				log.Printf("yt-dlp: %s", line)
			}
		}
	}()

	// Set headers for video streaming
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Cache-Control", "no-cache")

	// Stream the video directly to the response
	written, err := io.Copy(w, stdout)
	if err != nil {
		log.Printf("Error streaming video: %v", err)
		return
	}

	if err := cmd.Wait(); err != nil {
		log.Printf("yt-dlp process error: %v", err)
	}

	log.Printf("Streamed %d bytes", written)
}
