package mediaupload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

const (
	bucketName = "synapscribe-media"
	maxFileSize = 50 * 1024 * 1024 // 50MB
)

type MediaUploadResponse struct {
	FileURL    string    `json:"fileUrl"`
	FileName   string    `json:"fileName"`
	FileType   string    `json:"fileType"`
	UploadedAt time.Time `json:"uploadedAt"`
}

type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func init() {
	functions.HTTP("MediaUpload", MediaUpload)
}

func MediaUpload(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	err := r.ParseMultipartForm(maxFileSize)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Failed to parse form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "No file uploaded")
		return
	}
	defer file.Close()

	// Validate file type
	fileType := getFileType(header.Filename)
	if fileType == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Unsupported file type")
		return
	}

	// Upload to GCS
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to create storage client")
		return
	}
	defer client.Close()

	bucket := client.Bucket(bucketName)
	objectName := fmt.Sprintf("%s/%s", fileType, header.Filename)
	obj := bucket.Object(objectName)
	writer := obj.NewWriter(ctx)
	
	if _, err := io.Copy(writer, file); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to upload file")
		return
	}
	if err := writer.Close(); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to finalize upload")
		return
	}

	// Generate public URL for the uploaded file
	fileURL := fmt.Sprintf("https://storage.cloud.google.com/%s/%s", bucketName, objectName)

	response := MediaUploadResponse{
		FileURL:    fileURL,
		FileName:   header.Filename,
		FileType:   fileType,
		UploadedAt: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func getFileType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp3", ".wav", ".ogg":
		return "audio"
	case ".mp4", ".mov", ".avi":
		return "video"
	case ".jpg", ".jpeg", ".png", ".gif":
		return "image"
	default:
		return ""
	}
}

func sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Code:    statusCode,
		Message: message,
	})
}
