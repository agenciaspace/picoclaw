package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// GCalTool provides Google Calendar read/write capabilities via the Calendar REST API.
// Requires Google OAuth2 authentication with calendar scope.
type GCalTool struct {
	clientID     string
	clientSecret string
}

func NewGCalTool(clientID, clientSecret string) *GCalTool {
	return &GCalTool{
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (t *GCalTool) Name() string { return "gcal" }

func (t *GCalTool) Description() string {
	return "Manage Google Calendar events. Actions: list (list upcoming events), create (create a new event), search (search events by keyword)."
}

func (t *GCalTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list", "create", "search"},
				"description": "The action to perform: list, create, or search",
			},
			"calendar_id": map[string]interface{}{
				"type":        "string",
				"description": "Calendar ID (default: 'primary')",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Event title/summary (required for 'create' action)",
			},
			"start": map[string]interface{}{
				"type":        "string",
				"description": "Start time in RFC3339 format, e.g. '2026-02-20T09:00:00-03:00' (required for 'create')",
			},
			"end": map[string]interface{}{
				"type":        "string",
				"description": "End time in RFC3339 format (required for 'create')",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Event description (optional for 'create')",
			},
			"location": map[string]interface{}{
				"type":        "string",
				"description": "Event location (optional for 'create')",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query text (for 'search' action)",
			},
			"max_results": map[string]interface{}{
				"type":        "number",
				"description": "Maximum number of events to return (default 10, max 25)",
			},
			"days_ahead": map[string]interface{}{
				"type":        "number",
				"description": "Number of days ahead to list events (default 7, for 'list' action)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *GCalTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	case "list":
		return t.listEvents(ctx, args)
	case "create":
		return t.createEvent(ctx, args)
	case "search":
		return t.searchEvents(ctx, args)
	default:
		return ErrorResult("Invalid action. Use: list, create, or search")
	}
}

func (t *GCalTool) getToken() (string, error) {
	return auth.GetGoogleToken(t.clientID, t.clientSecret)
}

func (t *GCalTool) calendarID(args map[string]interface{}) string {
	if id, ok := args["calendar_id"].(string); ok && id != "" {
		return id
	}
	return "primary"
}

func (t *GCalTool) doRequest(ctx context.Context, method, reqURL string, body io.Reader) ([]byte, error) {
	token, err := t.getToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google Calendar API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading Calendar API response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Calendar API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

type calendarEvent struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Start       struct {
		DateTime string `json:"dateTime"`
		Date     string `json:"date"`
	} `json:"start"`
	End struct {
		DateTime string `json:"dateTime"`
		Date     string `json:"date"`
	} `json:"end"`
	HTMLLink string `json:"htmlLink"`
}

func formatEvent(e calendarEvent) string {
	startStr := e.Start.DateTime
	if startStr == "" {
		startStr = e.Start.Date + " (all day)"
	}
	endStr := e.End.DateTime
	if endStr == "" {
		endStr = e.End.Date
	}

	parts := []string{
		fmt.Sprintf("Title: %s", e.Summary),
		fmt.Sprintf("Start: %s", startStr),
		fmt.Sprintf("End: %s", endStr),
	}

	if e.Location != "" {
		parts = append(parts, fmt.Sprintf("Location: %s", e.Location))
	}
	if e.Description != "" {
		desc := e.Description
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		parts = append(parts, fmt.Sprintf("Description: %s", desc))
	}
	parts = append(parts, fmt.Sprintf("ID: %s", e.ID))

	return strings.Join(parts, "\n")
}

func (t *GCalTool) listEvents(ctx context.Context, args map[string]interface{}) *ToolResult {
	calID := t.calendarID(args)

	maxResults := 10
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
		if maxResults > 25 {
			maxResults = 25
		}
	}

	daysAhead := 7
	if d, ok := args["days_ahead"].(float64); ok && d > 0 {
		daysAhead = int(d)
	}

	now := time.Now()
	timeMin := now.Format(time.RFC3339)
	timeMax := now.AddDate(0, 0, daysAhead).Format(time.RFC3339)

	reqURL := fmt.Sprintf(
		"https://www.googleapis.com/calendar/v3/calendars/%s/events?maxResults=%d&timeMin=%s&timeMax=%s&singleEvents=true&orderBy=startTime",
		url.PathEscape(calID), maxResults, url.QueryEscape(timeMin), url.QueryEscape(timeMax),
	)

	data, err := t.doRequest(ctx, "GET", reqURL, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to list events: %v", err))
	}

	var listResp struct {
		Items   []calendarEvent `json:"items"`
		Summary string          `json:"summary"`
	}
	if err := json.Unmarshal(data, &listResp); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing calendar response: %v", err))
	}

	if len(listResp.Items) == 0 {
		return NewToolResult(fmt.Sprintf("No events in the next %d days.", daysAhead))
	}

	var results []string
	for _, event := range listResp.Items {
		results = append(results, formatEvent(event))
	}

	header := fmt.Sprintf("Calendar '%s': %d events in the next %d days\n\n", listResp.Summary, len(results), daysAhead)
	return NewToolResult(header + strings.Join(results, "\n---\n"))
}

func (t *GCalTool) createEvent(ctx context.Context, args map[string]interface{}) *ToolResult {
	calID := t.calendarID(args)
	title, _ := args["title"].(string)
	startStr, _ := args["start"].(string)
	endStr, _ := args["end"].(string)
	description, _ := args["description"].(string)
	location, _ := args["location"].(string)

	if title == "" || startStr == "" || endStr == "" {
		return ErrorResult("'title', 'start', and 'end' are required for create action")
	}

	// Validate time format
	if _, err := time.Parse(time.RFC3339, startStr); err != nil {
		return ErrorResult(fmt.Sprintf("Invalid 'start' time format. Use RFC3339 (e.g. 2026-02-20T09:00:00-03:00): %v", err))
	}
	if _, err := time.Parse(time.RFC3339, endStr); err != nil {
		return ErrorResult(fmt.Sprintf("Invalid 'end' time format. Use RFC3339 (e.g. 2026-02-20T10:00:00-03:00): %v", err))
	}

	event := map[string]interface{}{
		"summary": title,
		"start":   map[string]string{"dateTime": startStr},
		"end":     map[string]string{"dateTime": endStr},
	}
	if description != "" {
		event["description"] = description
	}
	if location != "" {
		event["location"] = location
	}

	payload, _ := json.Marshal(event)
	reqURL := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events", url.PathEscape(calID))

	data, err := t.doRequest(ctx, "POST", reqURL, strings.NewReader(string(payload)))
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to create event: %v", err))
	}

	var created calendarEvent
	if err := json.Unmarshal(data, &created); err != nil {
		logger.DebugCF("gcal", "Create event response parse error", map[string]interface{}{"error": err.Error()})
	}

	return NewToolResult(fmt.Sprintf("Event created successfully!\n\n%s\nLink: %s", formatEvent(created), created.HTMLLink))
}

func (t *GCalTool) searchEvents(ctx context.Context, args map[string]interface{}) *ToolResult {
	calID := t.calendarID(args)
	query, _ := args["query"].(string)

	if query == "" {
		return ErrorResult("'query' is required for search action")
	}

	maxResults := 10
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
		if maxResults > 25 {
			maxResults = 25
		}
	}

	now := time.Now()
	// Search within a wide range: 1 year back to 1 year ahead
	timeMin := now.AddDate(-1, 0, 0).Format(time.RFC3339)
	timeMax := now.AddDate(1, 0, 0).Format(time.RFC3339)

	reqURL := fmt.Sprintf(
		"https://www.googleapis.com/calendar/v3/calendars/%s/events?maxResults=%d&timeMin=%s&timeMax=%s&singleEvents=true&orderBy=startTime&q=%s",
		url.PathEscape(calID), maxResults, url.QueryEscape(timeMin), url.QueryEscape(timeMax), url.QueryEscape(query),
	)

	data, err := t.doRequest(ctx, "GET", reqURL, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to search events: %v", err))
	}

	var listResp struct {
		Items []calendarEvent `json:"items"`
	}
	if err := json.Unmarshal(data, &listResp); err != nil {
		return ErrorResult(fmt.Sprintf("Parsing search response: %v", err))
	}

	if len(listResp.Items) == 0 {
		return NewToolResult(fmt.Sprintf("No events found for query: %s", query))
	}

	var results []string
	for _, event := range listResp.Items {
		results = append(results, formatEvent(event))
	}

	header := fmt.Sprintf("Search '%s': %d events found\n\n", query, len(results))
	return NewToolResult(header + strings.Join(results, "\n---\n"))
}
