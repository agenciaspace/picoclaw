package tools

import (
	"context"
	"fmt"
)

type SendCallback func(channel, chatID, content string, media []string) error

type MessageTool struct {
	sendCallback   SendCallback
	defaultChannel string
	defaultChatID  string
	sentInRound    bool // Tracks whether a message was sent in the current processing round
}

func NewMessageTool() *MessageTool {
	return &MessageTool{}
}

func (t *MessageTool) Name() string {
	return "message"
}

func (t *MessageTool) Description() string {
	return "Send a message to user on a chat channel. Supports text and media (photos/files via file paths or URLs)."
}

func (t *MessageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The message content to send (text). Can be empty if only sending media.",
			},
			"media": map[string]interface{}{
				"type":        "array",
				"description": "Optional: list of file paths or URLs for photos/documents to send",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"channel": map[string]interface{}{
				"type":        "string",
				"description": "Optional: target channel (telegram, whatsapp, etc.)",
			},
			"chat_id": map[string]interface{}{
				"type":        "string",
				"description": "Optional: target chat/user ID",
			},
		},
		"required": []string{},
	}
}

func (t *MessageTool) SetContext(channel, chatID string) {
	t.defaultChannel = channel
	t.defaultChatID = chatID
	t.sentInRound = false // Reset send tracking for new processing round
}

// HasSentInRound returns true if the message tool sent a message during the current round.
func (t *MessageTool) HasSentInRound() bool {
	return t.sentInRound
}

func (t *MessageTool) SetSendCallback(callback SendCallback) {
	t.sendCallback = callback
}

func (t *MessageTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	content, _ := args["content"].(string)

	// Extract media array
	var media []string
	if mediaRaw, ok := args["media"].([]interface{}); ok {
		for _, m := range mediaRaw {
			if s, ok := m.(string); ok && s != "" {
				media = append(media, s)
			}
		}
	}

	if content == "" && len(media) == 0 {
		return &ToolResult{ForLLM: "content or media is required", IsError: true}
	}

	channel, _ := args["channel"].(string)
	chatID, _ := args["chat_id"].(string)

	if channel == "" {
		channel = t.defaultChannel
	}
	if chatID == "" {
		chatID = t.defaultChatID
	}

	if channel == "" || chatID == "" {
		return &ToolResult{ForLLM: "No target channel/chat specified", IsError: true}
	}

	if t.sendCallback == nil {
		return &ToolResult{ForLLM: "Message sending not configured", IsError: true}
	}

	if err := t.sendCallback(channel, chatID, content, media); err != nil {
		return &ToolResult{
			ForLLM:  fmt.Sprintf("sending message: %v", err),
			IsError: true,
			Err:     err,
		}
	}

	t.sentInRound = true
	// Silent: user already received the message directly
	result := fmt.Sprintf("Message sent to %s:%s", channel, chatID)
	if len(media) > 0 {
		result += fmt.Sprintf(" with %d media file(s)", len(media))
	}
	return &ToolResult{
		ForLLM: result,
		Silent: true,
	}
}
