package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// GDriveTool provides Google Drive file management capabilities via the Drive REST API.
// Requires Google OAuth2 authentication with drive.file or drive scopes.
type GDriveTool struct {
	clientID     string
	clientSecret string
}

func NewGDriveTool(clientID, clientSecret string) *GDriveTool {
	return &GDriveTool{
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (t *GDriveTool) Name() string { return "gdrive" }

func (t *GDriveTool) Description() string {
	return "Manage Google Drive files. Actions: list (list files), search (search files by name/content), read (read file content), upload (create a text file), info (get file metadata)."
}

func (t *GDriveTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list", "search", "read", "upload", "info"},
				"description": "The action to perform: list, search, read, upload, or info",
			},
			"file_id": map[string]interface{}{
				"type":        "string",
				"description": "File ID (required for 'read' and 'info' actions)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query (for 'search' action, e.g. 'name contains report')",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "File name (required for 'upload' action)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Text content to upload (required for 'upload' action)",
			},
			"mime_type": map[string]interface{}{
				"type":        "string",
				"description": "MIME type for upload (default: 'text/plain')",
			},
			"folder_id": map[string]interface{}{
				"type":        "string",
				"description": "Parent folder ID for upload or list (default: root)",
			},
			"max_results": map[string]interface{}{
				"type":        "number",
				"description": "Maximum number of results to return (default 10, max 25)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *GDriveTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	case "list":
		return t.listFiles(ctx, args)
	case "search":
		return t.searchFiles(ctx, args)
	case "read":
		return t.readFile(ctx, args)
	case "upload":
		return t.uploadFile(ctx, args)
	case "info":
		return t.fileInfo(ctx, args)
	default:
		return ErrorResult("Invalid action. Use: list, search, read, upload, or info")
	}
}

func (t *GDriveTool) getToken() (string, error) {
	return auth.GetGoogleToken(t.clientID, t.clientSecret)
}

func (t *GDriveTool) doRequest(ctx context.Context, method, reqURL string, body io.Reader, contentType string) ([]byte, error) {
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
		return nil, fmt.Errorf("Google Drive API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading Drive API response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Drive API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

type driveFile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	Size         string `json:"size"`
	ModifiedTime string `json:"modifiedTime"`
	WebViewLink  string `json:"webViewLink"`
	Parents      []string `json:"parents"`
}

func formatDriveFile(f driveFile) string {
	parts := []string{
		fmt.Sprintf("Name: %s", f.Name),
		fmt.Sprintf("ID: %s", f.ID),
		fmt.Sprintf("Type: %s", f.MimeType),
	}
	if f.Size != "" {
		parts = append(parts, fmt.Sprintf("Size: %s bytes", f.Size))
	}
	if f.ModifiedTime != "" {
		parts = append(parts, fmt.Sprintf("Modified: %s", f.ModifiedTime))
	}
	if f.WebViewLink != "" {
		parts = append(parts, fmt.Sprintf("Link: %s", f.WebViewLink))
	}
	return strings.Join(parts, "\n")
}

func (t *GDriveTool) listFiles(ctx context.Context, args map[string]interface{}) *ToolResult {
	maxResults := 10
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
		if maxResults > 25 {
			maxResults = 25
		}
	}

	q := "trashed=false"
	if folderID, ok := args["folder_id"].(string); ok && folderID != "" {
		q += fmt.Sprintf(" and '%s' in parents", folderID)
	}

	reqURL := fmt.Sprintf(
		"https://www.googleapis.com/drive/v3/files?pageSize=%d&q=%s&fields=files(id,name,mimeType,size,modifiedTime,webViewLink,parents)&orderBy=modifiedTime desc",
		maxResults, url.QueryEscape(q),
	)

	data, err := t.doRequest(ctx, "GET", reqURL, nil, "")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to list files: %v", err))
	}

	var listResp struct {
		Files []driveFile `json:"files"`
	}
	if err := json.Unmarshal(data, &listResp); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing Drive response: %v", err))
	}

	if len(listResp.Files) == 0 {
		return NewToolResult("No files found.")
	}

	var results []string
	for _, f := range listResp.Files {
		results = append(results, formatDriveFile(f))
	}

	header := fmt.Sprintf("Google Drive: %d files\n\n", len(results))
	return NewToolResult(header + strings.Join(results, "\n---\n"))
}

func (t *GDriveTool) searchFiles(ctx context.Context, args map[string]interface{}) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return ErrorResult("'query' is required for search action (e.g. \"name contains 'report'\")")
	}

	maxResults := 10
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
		if maxResults > 25 {
			maxResults = 25
		}
	}

	// Combine user query with trashed filter
	fullQuery := fmt.Sprintf("(%s) and trashed=false", query)

	reqURL := fmt.Sprintf(
		"https://www.googleapis.com/drive/v3/files?pageSize=%d&q=%s&fields=files(id,name,mimeType,size,modifiedTime,webViewLink)&orderBy=modifiedTime desc",
		maxResults, url.QueryEscape(fullQuery),
	)

	data, err := t.doRequest(ctx, "GET", reqURL, nil, "")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to search files: %v", err))
	}

	var listResp struct {
		Files []driveFile `json:"files"`
	}
	if err := json.Unmarshal(data, &listResp); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing search response: %v", err))
	}

	if len(listResp.Files) == 0 {
		return NewToolResult(fmt.Sprintf("No files found for query: %s", query))
	}

	var results []string
	for _, f := range listResp.Files {
		results = append(results, formatDriveFile(f))
	}

	header := fmt.Sprintf("Search '%s': %d files found\n\n", query, len(results))
	return NewToolResult(header + strings.Join(results, "\n---\n"))
}

func (t *GDriveTool) readFile(ctx context.Context, args map[string]interface{}) *ToolResult {
	fileID, _ := args["file_id"].(string)
	if fileID == "" {
		return ErrorResult("'file_id' is required for read action")
	}

	// First get file metadata to check type
	metaURL := fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s?fields=id,name,mimeType,size", url.PathEscape(fileID))
	metaData, err := t.doRequest(ctx, "GET", metaURL, nil, "")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to get file info: %v", err))
	}

	var fileMeta driveFile
	if err := json.Unmarshal(metaData, &fileMeta); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing file metadata: %v", err))
	}

	// For Google Docs/Sheets/Slides, use export
	var contentURL string
	if strings.HasPrefix(fileMeta.MimeType, "application/vnd.google-apps.") {
		exportMime := "text/plain"
		switch fileMeta.MimeType {
		case "application/vnd.google-apps.spreadsheet":
			exportMime = "text/csv"
		case "application/vnd.google-apps.presentation":
			exportMime = "text/plain"
		}
		contentURL = fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s/export?mimeType=%s",
			url.PathEscape(fileID), url.QueryEscape(exportMime))
	} else {
		contentURL = fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s?alt=media", url.PathEscape(fileID))
	}

	content, err := t.doRequest(ctx, "GET", contentURL, nil, "")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to read file content: %v", err))
	}

	// Truncate very large files
	text := string(content)
	if len(text) > 50000 {
		text = text[:50000] + "\n\n[... content truncated at 50000 characters]"
	}

	header := fmt.Sprintf("File: %s (%s)\n\n", fileMeta.Name, fileMeta.MimeType)
	return NewToolResult(header + text)
}

func (t *GDriveTool) uploadFile(ctx context.Context, args map[string]interface{}) *ToolResult {
	name, _ := args["name"].(string)
	content, _ := args["content"].(string)

	if name == "" || content == "" {
		return ErrorResult("'name' and 'content' are required for upload action")
	}

	mimeType, _ := args["mime_type"].(string)
	if mimeType == "" {
		mimeType = "text/plain"
	}

	// Build metadata
	metadata := map[string]interface{}{
		"name":     name,
		"mimeType": mimeType,
	}
	if folderID, ok := args["folder_id"].(string); ok && folderID != "" {
		metadata["parents"] = []string{folderID}
	}

	metadataJSON, _ := json.Marshal(metadata)

	// Use multipart upload for simplicity
	boundary := "picoclaw_boundary_upload"
	var body strings.Builder
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Type: application/json; charset=UTF-8\r\n\r\n")
	body.WriteString(string(metadataJSON))
	body.WriteString(fmt.Sprintf("\r\n--%s\r\n", boundary))
	body.WriteString(fmt.Sprintf("Content-Type: %s\r\n\r\n", mimeType))
	body.WriteString(content)
	body.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))

	reqURL := "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart&fields=id,name,mimeType,webViewLink"
	contentTypeHeader := fmt.Sprintf("multipart/related; boundary=%s", boundary)

	data, err := t.doRequest(ctx, "POST", reqURL, strings.NewReader(body.String()), contentTypeHeader)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to upload file: %v", err))
	}

	var created driveFile
	if err := json.Unmarshal(data, &created); err != nil {
		logger.DebugCF("gdrive", "Upload response parse error", map[string]interface{}{"error": err.Error()})
	}

	result := fmt.Sprintf("File uploaded successfully!\n\nName: %s\nID: %s\nType: %s", created.Name, created.ID, created.MimeType)
	if created.WebViewLink != "" {
		result += fmt.Sprintf("\nLink: %s", created.WebViewLink)
	}
	return NewToolResult(result)
}

func (t *GDriveTool) fileInfo(ctx context.Context, args map[string]interface{}) *ToolResult {
	fileID, _ := args["file_id"].(string)
	if fileID == "" {
		return ErrorResult("'file_id' is required for info action")
	}

	reqURL := fmt.Sprintf(
		"https://www.googleapis.com/drive/v3/files/%s?fields=id,name,mimeType,size,modifiedTime,createdTime,webViewLink,parents,shared,owners",
		url.PathEscape(fileID),
	)

	data, err := t.doRequest(ctx, "GET", reqURL, nil, "")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to get file info: %v", err))
	}

	var file struct {
		driveFile
		CreatedTime string `json:"createdTime"`
		Shared      bool   `json:"shared"`
		Owners      []struct {
			DisplayName  string `json:"displayName"`
			EmailAddress string `json:"emailAddress"`
		} `json:"owners"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing file info: %v", err))
	}

	parts := []string{
		fmt.Sprintf("Name: %s", file.Name),
		fmt.Sprintf("ID: %s", file.ID),
		fmt.Sprintf("Type: %s", file.MimeType),
	}
	if file.Size != "" {
		parts = append(parts, fmt.Sprintf("Size: %s bytes", file.Size))
	}
	if file.CreatedTime != "" {
		parts = append(parts, fmt.Sprintf("Created: %s", file.CreatedTime))
	}
	if file.ModifiedTime != "" {
		parts = append(parts, fmt.Sprintf("Modified: %s", file.ModifiedTime))
	}
	parts = append(parts, fmt.Sprintf("Shared: %t", file.Shared))
	if len(file.Owners) > 0 {
		parts = append(parts, fmt.Sprintf("Owner: %s (%s)", file.Owners[0].DisplayName, file.Owners[0].EmailAddress))
	}
	if file.WebViewLink != "" {
		parts = append(parts, fmt.Sprintf("Link: %s", file.WebViewLink))
	}

	return NewToolResult(strings.Join(parts, "\n"))
}
