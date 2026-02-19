package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// GmailTool provides Gmail read/send capabilities via the Gmail REST API.
// Requires Google OAuth2 authentication with gmail scopes.
type GmailTool struct {
	clientID     string
	clientSecret string
}

func NewGmailTool(clientID, clientSecret string) *GmailTool {
	return &GmailTool{
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (t *GmailTool) Name() string { return "gmail" }

func (t *GmailTool) Description() string {
	return "Read and send Gmail emails. Actions: list (list recent emails), read (read a specific email), send (send an email), search (search emails by query)."
}

func (t *GmailTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list", "read", "send", "search"},
				"description": "The action to perform: list, read, send, or search",
			},
			"message_id": map[string]interface{}{
				"type":        "string",
				"description": "Message ID (required for 'read' action)",
			},
			"to": map[string]interface{}{
				"type":        "string",
				"description": "Recipient email address (required for 'send' action)",
			},
			"subject": map[string]interface{}{
				"type":        "string",
				"description": "Email subject (required for 'send' action)",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Email body text (required for 'send' action)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Gmail search query (for 'search' action, e.g. 'from:user@example.com is:unread')",
			},
			"max_results": map[string]interface{}{
				"type":        "number",
				"description": "Maximum number of results to return (default 10, max 20)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *GmailTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	case "list":
		return t.listMessages(ctx, args)
	case "read":
		return t.readMessage(ctx, args)
	case "send":
		return t.sendMessage(ctx, args)
	case "search":
		return t.searchMessages(ctx, args)
	default:
		return ErrorResult("Invalid action. Use: list, read, send, or search")
	}
}

func (t *GmailTool) getToken() (string, error) {
	return auth.GetGoogleToken(t.clientID, t.clientSecret)
}

func (t *GmailTool) doRequest(ctx context.Context, method, url string, body io.Reader) ([]byte, error) {
	token, err := t.getToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Gmail API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading Gmail API response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Gmail API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (t *GmailTool) listMessages(ctx context.Context, args map[string]interface{}) *ToolResult {
	maxResults := 10
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
		if maxResults > 20 {
			maxResults = 20
		}
	}

	url := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages?maxResults=%d&labelIds=INBOX", maxResults)
	data, err := t.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to list messages: %v", err))
	}

	var listResp struct {
		Messages []struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		} `json:"messages"`
		ResultSizeEstimate int `json:"resultSizeEstimate"`
	}
	if err := json.Unmarshal(data, &listResp); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing Gmail response: %v", err))
	}

	if len(listResp.Messages) == 0 {
		return NewToolResult("No messages found in inbox.")
	}

	// Fetch headers for each message
	var results []string
	for _, msg := range listResp.Messages {
		summary := t.getMessageSummary(ctx, msg.ID)
		results = append(results, summary)
	}

	header := fmt.Sprintf("Inbox: %d messages (showing %d)\n\n", listResp.ResultSizeEstimate, len(results))
	return NewToolResult(header + strings.Join(results, "\n---\n"))
}

func (t *GmailTool) getMessageSummary(ctx context.Context, messageID string) string {
	url := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages/%s?format=metadata&metadataHeaders=From&metadataHeaders=Subject&metadataHeaders=Date", messageID)
	data, err := t.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Sprintf("[%s] (failed to fetch)", messageID)
	}

	var msg struct {
		ID      string `json:"id"`
		Payload struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
		} `json:"payload"`
		Snippet string `json:"snippet"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Sprintf("[%s] (parse error)", messageID)
	}

	from, subject, date := "", "", ""
	for _, h := range msg.Payload.Headers {
		switch h.Name {
		case "From":
			from = h.Value
		case "Subject":
			subject = h.Value
		case "Date":
			date = h.Value
		}
	}

	return fmt.Sprintf("ID: %s\nFrom: %s\nSubject: %s\nDate: %s\nPreview: %s", msg.ID, from, subject, date, msg.Snippet)
}

func (t *GmailTool) readMessage(ctx context.Context, args map[string]interface{}) *ToolResult {
	messageID, _ := args["message_id"].(string)
	if messageID == "" {
		return ErrorResult("message_id is required for read action")
	}

	url := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages/%s?format=full", messageID)
	data, err := t.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to read message: %v", err))
	}

	var msg struct {
		ID      string `json:"id"`
		Payload struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
			MimeType string `json:"mimeType"`
			Body     struct {
				Data string `json:"data"`
			} `json:"body"`
			Parts []struct {
				MimeType string `json:"mimeType"`
				Body     struct {
					Data string `json:"data"`
				} `json:"body"`
			} `json:"parts"`
		} `json:"payload"`
		Snippet string `json:"snippet"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing message: %v", err))
	}

	from, subject, date, to := "", "", "", ""
	for _, h := range msg.Payload.Headers {
		switch h.Name {
		case "From":
			from = h.Value
		case "To":
			to = h.Value
		case "Subject":
			subject = h.Value
		case "Date":
			date = h.Value
		}
	}

	// Extract body text
	bodyText := extractBodyText(msg.Payload.Body.Data, msg.Payload.Parts)

	result := fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\nDate: %s\n\n%s", from, to, subject, date, bodyText)
	return NewToolResult(result)
}

func extractBodyText(bodyData string, parts []struct {
	MimeType string `json:"mimeType"`
	Body     struct {
		Data string `json:"data"`
	} `json:"body"`
}) string {
	// Try direct body first
	if bodyData != "" {
		if decoded, err := base64.URLEncoding.DecodeString(bodyData); err == nil {
			return string(decoded)
		}
	}

	// Try parts
	for _, part := range parts {
		if part.MimeType == "text/plain" && part.Body.Data != "" {
			if decoded, err := base64.URLEncoding.DecodeString(part.Body.Data); err == nil {
				return string(decoded)
			}
		}
	}

	// Fallback to HTML part
	for _, part := range parts {
		if part.MimeType == "text/html" && part.Body.Data != "" {
			if decoded, err := base64.URLEncoding.DecodeString(part.Body.Data); err == nil {
				return "[HTML content]\n" + string(decoded)
			}
		}
	}

	return "(no text content)"
}

func (t *GmailTool) sendMessage(ctx context.Context, args map[string]interface{}) *ToolResult {
	to, _ := args["to"].(string)
	subject, _ := args["subject"].(string)
	body, _ := args["body"].(string)

	if to == "" || subject == "" || body == "" {
		return ErrorResult("'to', 'subject', and 'body' are required for send action")
	}

	// Build RFC 2822 message
	raw := fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", to, subject, body)
	encoded := base64.URLEncoding.EncodeToString([]byte(raw))

	payload := map[string]string{"raw": encoded}
	payloadJSON, _ := json.Marshal(payload)

	url := "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"
	data, err := t.doRequest(ctx, "POST", url, strings.NewReader(string(payloadJSON)))
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to send email: %v", err))
	}

	var sendResp struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(data, &sendResp); err != nil {
		logger.DebugCF("gmail", "Send response parse error", map[string]interface{}{"error": err.Error()})
	}

	return NewToolResult(fmt.Sprintf("Email sent successfully to %s (ID: %s)", to, sendResp.ID))
}

func (t *GmailTool) searchMessages(ctx context.Context, args map[string]interface{}) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return ErrorResult("'query' is required for search action (e.g. 'from:user@example.com is:unread')")
	}

	maxResults := 10
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
		if maxResults > 20 {
			maxResults = 20
		}
	}

	url := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages?maxResults=%d&q=%s", maxResults, query)
	data, err := t.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to search messages: %v", err))
	}

	var listResp struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		ResultSizeEstimate int `json:"resultSizeEstimate"`
	}
	if err := json.Unmarshal(data, &listResp); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing search response: %v", err))
	}

	if len(listResp.Messages) == 0 {
		return NewToolResult(fmt.Sprintf("No messages found for query: %s", query))
	}

	var results []string
	for _, msg := range listResp.Messages {
		summary := t.getMessageSummary(ctx, msg.ID)
		results = append(results, summary)
	}

	header := fmt.Sprintf("Search '%s': %d results (showing %d)\n\n", query, listResp.ResultSizeEstimate, len(results))
	return NewToolResult(header + strings.Join(results, "\n---\n"))
}
