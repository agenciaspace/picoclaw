package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// GPhotosTool provides Google Photos management capabilities via the Google Photos Library API.
// Supports listing, searching, downloading, and uploading photos.
// Requires OAuth2 with photoslibrary scope.
type GPhotosTool struct {
	clientID     string
	clientSecret string
}

func NewGPhotosTool(clientID, clientSecret string) *GPhotosTool {
	return &GPhotosTool{
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (t *GPhotosTool) Name() string { return "gphotos" }

func (t *GPhotosTool) Description() string {
	return "Manage Google Photos. Actions: list (list recent photos), search (search by date/category), download (download a photo to local file), upload (upload a local file to Google Photos), albums (list albums)."
}

func (t *GPhotosTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list", "search", "download", "upload", "albums"},
				"description": "The action to perform",
			},
			"item_id": map[string]interface{}{
				"type":        "string",
				"description": "Media item ID (required for 'download' action)",
			},
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "Local file path: destination for download, source for upload",
			},
			"album_id": map[string]interface{}{
				"type":        "string",
				"description": "Album ID to list photos from or upload to",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query for 'search' action. Supports category filters like 'selfies', 'landscapes', 'pets', etc.",
			},
			"date_filter": map[string]interface{}{
				"type":        "string",
				"description": "Date filter for 'search' in YYYY-MM-DD format (searches photos from that date)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Description for uploaded photo",
			},
			"max_results": map[string]interface{}{
				"type":        "number",
				"description": "Maximum number of results (default 10, max 25)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *GPhotosTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	case "list":
		return t.listPhotos(ctx, args)
	case "search":
		return t.searchPhotos(ctx, args)
	case "download":
		return t.downloadPhoto(ctx, args)
	case "upload":
		return t.uploadPhoto(ctx, args)
	case "albums":
		return t.listAlbums(ctx, args)
	default:
		return ErrorResult("Invalid action. Use: list, search, download, upload, or albums")
	}
}

func (t *GPhotosTool) getToken() (string, error) {
	return auth.GetGoogleToken(t.clientID, t.clientSecret)
}

func (t *GPhotosTool) doRequest(ctx context.Context, method, reqURL string, body io.Reader, contentType string) ([]byte, error) {
	token, err := t.getToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google Photos API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading Photos API response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Photos API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

type mediaItem struct {
	ID            string `json:"id"`
	Description   string `json:"description"`
	ProductURL    string `json:"productUrl"`
	BaseURL       string `json:"baseUrl"`
	MimeType      string `json:"mimeType"`
	Filename      string `json:"filename"`
	MediaMetadata struct {
		CreationTime string `json:"creationTime"`
		Width        string `json:"width"`
		Height       string `json:"height"`
		Photo        *struct {
			CameraMake  string  `json:"cameraMake"`
			CameraModel string  `json:"cameraModel"`
			FocalLength float64 `json:"focalLength"`
		} `json:"photo"`
		Video *struct {
			CameraMake  string `json:"cameraMake"`
			CameraModel string `json:"cameraModel"`
			Fps         float64 `json:"fps"`
			Status      string `json:"status"`
		} `json:"video"`
	} `json:"mediaMetadata"`
}

func formatMediaItem(item mediaItem) string {
	parts := []string{
		fmt.Sprintf("Filename: %s", item.Filename),
		fmt.Sprintf("ID: %s", item.ID),
		fmt.Sprintf("Type: %s", item.MimeType),
	}
	meta := item.MediaMetadata
	if meta.Width != "" && meta.Height != "" {
		parts = append(parts, fmt.Sprintf("Size: %sx%s", meta.Width, meta.Height))
	}
	if meta.CreationTime != "" {
		parts = append(parts, fmt.Sprintf("Created: %s", meta.CreationTime))
	}
	if item.Description != "" {
		parts = append(parts, fmt.Sprintf("Description: %s", item.Description))
	}
	if meta.Photo != nil && meta.Photo.CameraModel != "" {
		parts = append(parts, fmt.Sprintf("Camera: %s %s", meta.Photo.CameraMake, meta.Photo.CameraModel))
	}
	if meta.Video != nil {
		parts = append(parts, "Media type: Video")
	}
	return strings.Join(parts, "\n")
}

func (t *GPhotosTool) listPhotos(ctx context.Context, args map[string]interface{}) *ToolResult {
	pageSize := 10
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		pageSize = int(mr)
		if pageSize > 25 {
			pageSize = 25
		}
	}

	albumID, _ := args["album_id"].(string)

	// Use mediaItems:search for album filtering, otherwise list
	if albumID != "" {
		payload := map[string]interface{}{
			"albumId":  albumID,
			"pageSize": pageSize,
		}
		payloadJSON, _ := json.Marshal(payload)

		data, err := t.doRequest(ctx, "POST",
			"https://photoslibrary.googleapis.com/v1/mediaItems:search",
			bytes.NewReader(payloadJSON), "application/json")
		if err != nil {
			return ErrorResult(fmt.Sprintf("Failed to list album photos: %v", err))
		}
		return t.formatMediaList(data, "Album photos")
	}

	reqURL := fmt.Sprintf("https://photoslibrary.googleapis.com/v1/mediaItems?pageSize=%d", pageSize)
	data, err := t.doRequest(ctx, "GET", reqURL, nil, "")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to list photos: %v", err))
	}
	return t.formatMediaList(data, "Recent photos")
}

func (t *GPhotosTool) searchPhotos(ctx context.Context, args map[string]interface{}) *ToolResult {
	pageSize := 10
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		pageSize = int(mr)
		if pageSize > 25 {
			pageSize = 25
		}
	}

	payload := map[string]interface{}{
		"pageSize": pageSize,
	}

	filters := map[string]interface{}{}

	// Category filter from query
	if query, ok := args["query"].(string); ok && query != "" {
		categoryMap := map[string]string{
			"selfies":    "SELFIES",
			"people":     "PEOPLE",
			"landscapes": "LANDSCAPES",
			"pets":       "PETS",
			"animals":    "ANIMALS",
			"food":       "FOOD",
			"travel":     "TRAVEL",
			"cityscapes": "CITYSCAPES",
			"screenshots": "SCREENSHOTS",
			"receipts":   "RECEIPTS",
			"documents":  "DOCUMENTS",
			"whiteboards": "WHITEBOARDS",
		}
		lq := strings.ToLower(query)
		if cat, found := categoryMap[lq]; found {
			filters["contentFilter"] = map[string]interface{}{
				"includedContentCategories": []string{cat},
			}
		}
	}

	// Date filter
	if dateStr, ok := args["date_filter"].(string); ok && dateStr != "" {
		parts := strings.Split(dateStr, "-")
		if len(parts) == 3 {
			filters["dateFilter"] = map[string]interface{}{
				"dates": []map[string]interface{}{
					{
						"year":  parts[0],
						"month": parts[1],
						"day":   parts[2],
					},
				},
			}
		}
	}

	// Media type filter - always include photos and videos
	if _, hasContent := filters["contentFilter"]; !hasContent {
		filters["mediaTypeFilter"] = map[string]interface{}{
			"mediaTypes": []string{"ALL_MEDIA"},
		}
	}

	if len(filters) > 0 {
		payload["filters"] = filters
	}

	payloadJSON, _ := json.Marshal(payload)

	data, err := t.doRequest(ctx, "POST",
		"https://photoslibrary.googleapis.com/v1/mediaItems:search",
		bytes.NewReader(payloadJSON), "application/json")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to search photos: %v", err))
	}

	queryLabel := ""
	if q, ok := args["query"].(string); ok {
		queryLabel = q
	}
	if d, ok := args["date_filter"].(string); ok && d != "" {
		if queryLabel != "" {
			queryLabel += " "
		}
		queryLabel += "date:" + d
	}
	return t.formatMediaList(data, fmt.Sprintf("Search '%s'", queryLabel))
}

func (t *GPhotosTool) formatMediaList(data []byte, header string) *ToolResult {
	var resp struct {
		MediaItems    []mediaItem `json:"mediaItems"`
		NextPageToken string      `json:"nextPageToken"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing response: %v", err))
	}

	if len(resp.MediaItems) == 0 {
		return NewToolResult(fmt.Sprintf("%s: no items found.", header))
	}

	var results []string
	for _, item := range resp.MediaItems {
		results = append(results, formatMediaItem(item))
	}

	return NewToolResult(fmt.Sprintf("%s: %d items\n\n%s", header, len(results), strings.Join(results, "\n---\n")))
}

func (t *GPhotosTool) downloadPhoto(ctx context.Context, args map[string]interface{}) *ToolResult {
	itemID, _ := args["item_id"].(string)
	if itemID == "" {
		return ErrorResult("'item_id' is required for download action")
	}

	// Get media item to find baseUrl
	reqURL := fmt.Sprintf("https://photoslibrary.googleapis.com/v1/mediaItems/%s", url.PathEscape(itemID))
	data, err := t.doRequest(ctx, "GET", reqURL, nil, "")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to get photo info: %v", err))
	}

	var item mediaItem
	if err := json.Unmarshal(data, &item); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing photo info: %v", err))
	}

	if item.BaseURL == "" {
		return ErrorResult("Photo has no download URL available")
	}

	// Append =d for full resolution download
	downloadURL := item.BaseURL + "=d"
	if item.MediaMetadata.Video != nil {
		downloadURL = item.BaseURL + "=dv"
	}

	// Determine output path
	destPath, _ := args["file_path"].(string)
	if destPath == "" {
		destPath = filepath.Join(os.TempDir(), "gphotos_"+item.Filename)
	}

	// Download the file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Downloading photo: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ErrorResult(fmt.Sprintf("Download failed (HTTP %d)", resp.StatusCode))
	}

	outFile, err := os.Create(destPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Creating output file: %v", err))
	}
	defer outFile.Close()

	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Writing photo to disk: %v", err))
	}

	result := fmt.Sprintf("Photo downloaded successfully!\n\nFilename: %s\nSaved to: %s\nSize: %d bytes\nType: %s",
		item.Filename, destPath, written, item.MimeType)
	return NewToolResult(result)
}

func (t *GPhotosTool) uploadPhoto(ctx context.Context, args map[string]interface{}) *ToolResult {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return ErrorResult("'file_path' is required for upload action")
	}

	// Read the file
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Reading file %s: %v", filePath, err))
	}

	filename := filepath.Base(filePath)

	// Detect mime type from extension
	mimeType := "application/octet-stream"
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".png":
		mimeType = "image/png"
	case ".gif":
		mimeType = "image/gif"
	case ".webp":
		mimeType = "image/webp"
	case ".heic":
		mimeType = "image/heic"
	case ".mp4":
		mimeType = "video/mp4"
	case ".mov":
		mimeType = "video/quicktime"
	}

	// Step 1: Upload bytes to get upload token
	token, err := t.getToken()
	if err != nil {
		return ErrorResult(fmt.Sprintf("Getting auth token: %v", err))
	}

	uploadReq, err := http.NewRequestWithContext(ctx, "POST",
		"https://photoslibrary.googleapis.com/v1/uploads",
		bytes.NewReader(fileData))
	if err != nil {
		return ErrorResult(fmt.Sprintf("Creating upload request: %v", err))
	}
	uploadReq.Header.Set("Authorization", "Bearer "+token)
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	uploadReq.Header.Set("X-Goog-Upload-Content-Type", mimeType)
	uploadReq.Header.Set("X-Goog-Upload-Protocol", "raw")

	uploadResp, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Upload request failed: %v", err))
	}
	defer uploadResp.Body.Close()

	uploadToken, err := io.ReadAll(uploadResp.Body)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Reading upload token: %v", err))
	}

	if uploadResp.StatusCode >= 400 {
		return ErrorResult(fmt.Sprintf("Upload failed (%d): %s", uploadResp.StatusCode, string(uploadToken)))
	}

	// Step 2: Create media item with the upload token
	description, _ := args["description"].(string)
	newMediaItem := map[string]interface{}{
		"description":    description,
		"simpleMediaItem": map[string]interface{}{
			"fileName":    filename,
			"uploadToken": string(uploadToken),
		},
	}

	createPayload := map[string]interface{}{
		"newMediaItems": []interface{}{newMediaItem},
	}

	if albumID, ok := args["album_id"].(string); ok && albumID != "" {
		createPayload["albumId"] = albumID
	}

	payloadJSON, _ := json.Marshal(createPayload)

	createData, err := t.doRequest(ctx, "POST",
		"https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate",
		bytes.NewReader(payloadJSON), "application/json")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Creating media item: %v", err))
	}

	var createResp struct {
		NewMediaItemResults []struct {
			UploadToken string `json:"uploadToken"`
			Status      struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"status"`
			MediaItem mediaItem `json:"mediaItem"`
		} `json:"newMediaItemResults"`
	}
	if err := json.Unmarshal(createData, &createResp); err != nil {
		logger.DebugCF("gphotos", "Create response parse error", map[string]interface{}{"error": err.Error()})
		return NewToolResult("Photo uploaded (could not parse response details)")
	}

	if len(createResp.NewMediaItemResults) == 0 {
		return ErrorResult("Upload returned no results")
	}

	r := createResp.NewMediaItemResults[0]
	if r.Status.Code != 0 {
		return ErrorResult(fmt.Sprintf("Upload error: %s (code %d)", r.Status.Message, r.Status.Code))
	}

	result := fmt.Sprintf("Photo uploaded to Google Photos!\n\nFilename: %s\nID: %s\nURL: %s",
		r.MediaItem.Filename, r.MediaItem.ID, r.MediaItem.ProductURL)
	return NewToolResult(result)
}

func (t *GPhotosTool) listAlbums(ctx context.Context, args map[string]interface{}) *ToolResult {
	pageSize := 10
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		pageSize = int(mr)
		if pageSize > 25 {
			pageSize = 25
		}
	}

	reqURL := fmt.Sprintf("https://photoslibrary.googleapis.com/v1/albums?pageSize=%d", pageSize)
	data, err := t.doRequest(ctx, "GET", reqURL, nil, "")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to list albums: %v", err))
	}

	var resp struct {
		Albums []struct {
			ID                    string `json:"id"`
			Title                 string `json:"title"`
			ProductURL            string `json:"productUrl"`
			MediaItemsCount       string `json:"mediaItemsCount"`
			CoverPhotoBaseURL     string `json:"coverPhotoBaseUrl"`
			CoverPhotoMediaItemID string `json:"coverPhotoMediaItemId"`
		} `json:"albums"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing albums response: %v", err))
	}

	if len(resp.Albums) == 0 {
		return NewToolResult("No albums found.")
	}

	var results []string
	for _, a := range resp.Albums {
		parts := []string{
			fmt.Sprintf("Title: %s", a.Title),
			fmt.Sprintf("ID: %s", a.ID),
		}
		if a.MediaItemsCount != "" {
			parts = append(parts, fmt.Sprintf("Items: %s", a.MediaItemsCount))
		}
		if a.ProductURL != "" {
			parts = append(parts, fmt.Sprintf("URL: %s", a.ProductURL))
		}
		results = append(results, strings.Join(parts, "\n"))
	}

	header := fmt.Sprintf("Google Photos: %d albums\n\n", len(results))
	return NewToolResult(header + strings.Join(results, "\n---\n"))
}
