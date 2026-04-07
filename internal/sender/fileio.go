package sender

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
)

const fileUploadURL = "https://file.io"

type fileUploadResult struct {
	Success bool   `json:"success"`
	Link    string `json:"link"`
	Message string `json:"message"`
}

// FileUploader uploads files to file.io and returns the download URL.
type FileUploader struct {
	client *http.Client
}

// NewFileUploader creates a new FileUploader.
func NewFileUploader() *FileUploader {
	return &FileUploader{
		client: &http.Client{Timeout: 60 * 10}, // 10 min timeout for large files
	}
}

// UploadFile uploads a local file to file.io and returns the download URL.
// Returns error if file is larger than maxSizeBytes (pass 0 to skip check).
func (f *FileUploader) UploadFile(filePath string, maxSizeBytes int64) (string, error) {
	if maxSizeBytes > 0 {
		info, err := os.Stat(filePath)
		if err != nil {
			return "", fmt.Errorf("can't stat file: %w", err)
		}
		if info.Size() > maxSizeBytes {
			return "", fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxSizeBytes)
		}
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("can't open file: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filePath)
	if err != nil {
		return "", fmt.Errorf("can't create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("can't copy file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("can't close writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, fileUploadURL, &buf)
	if err != nil {
		return "", fmt.Errorf("can't create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("can't send request: %w", err)
	}
	defer resp.Body.Close()

	var result fileUploadResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("can't decode response: %w", err)
	}

	if !result.Success {
		return "", fmt.Errorf("upload failed: %s", result.Message)
	}

	return result.Link, nil
}

// UploadReader uploads content from a reader to file.io and returns the download URL.
func (f *FileUploader) UploadReader(reader io.Reader, filename string, maxSizeBytes int64) (string, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("can't read data: %w", err)
	}

	if maxSizeBytes > 0 && int64(len(data)) > maxSizeBytes {
		return "", fmt.Errorf("data too large: %d bytes (max %d)", len(data), maxSizeBytes)
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("can't create form file: %w", err)
	}

	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("can't write data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("can't close writer: %w", err)
	}

	req, err := http.NewRequest("POST", "https://file.io", &buf)
	if err != nil {
		return "", fmt.Errorf("can't create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("can't send request: %w", err)
	}
	defer resp.Body.Close()

	var result fileUploadResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("can't decode response: %w", err)
	}

	if !result.Success {
		return "", fmt.Errorf("upload failed: %s", result.Message)
	}

	return result.Link, nil
}
