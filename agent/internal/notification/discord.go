package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DiscordModule implements the NotificationModule interface for Discord webhooks
type DiscordModule struct{}

// NewDiscordModule creates a new Discord notification module
func NewDiscordModule() *DiscordModule {
	return &DiscordModule{}
}

// Name returns the notification type identifier
func (d *DiscordModule) Name() string {
	return "discord"
}

// Send delivers a notification to Discord using a webhook URL
func (d *DiscordModule) Send(ctx context.Context, url string, payload NotificationPayload) error {
	// Format the Discord webhook payload
	webhookPayload := d.formatWebhookPayload(payload)

	// Marshal to JSON
	jsonData, err := json.Marshal(webhookPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord webhook payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create Discord webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Discord webhook: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Discord webhook returned non-success status: %d", resp.StatusCode)
	}

	return nil
}

// formatWebhookPayload formats the notification payload as a Discord webhook message
func (d *DiscordModule) formatWebhookPayload(payload NotificationPayload) map[string]interface{} {
	// Determine color based on event type
	color := d.getColorForEvent(payload.Event)

	// Build embed fields
	fields := []map[string]interface{}{
		{
			"name":   "Node",
			"value":  payload.NodeName,
			"inline": true,
		},
		{
			"name":   "Event",
			"value":  string(payload.Event),
			"inline": true,
		},
		{
			"name":   "Timestamp",
			"value":  payload.Timestamp.Format(time.RFC3339),
			"inline": false,
		},
	}

	// Add detail fields
	for key, value := range payload.Details {
		fields = append(fields, map[string]interface{}{
			"name":   key,
			"value":  fmt.Sprintf("%v", value),
			"inline": true,
		})
	}

	// Build the embed
	embed := map[string]interface{}{
		"title":       d.getTitleForEvent(payload.Event),
		"description": payload.Message,
		"color":       color,
		"fields":      fields,
		"timestamp":   payload.Timestamp.Format(time.RFC3339),
	}

	return map[string]interface{}{
		"embeds": []map[string]interface{}{embed},
	}
}

// getColorForEvent returns the Discord embed color for an event type
func (d *DiscordModule) getColorForEvent(event NotificationEvent) int {
	switch event {
	case EventFailure:
		return 0xFF0000 // Red
	case EventSkip:
		return 0xFFA500 // Orange
	case EventComplete:
		return 0x00FF00 // Green
	default:
		return 0x808080 // Gray
	}
}

// getTitleForEvent returns the Discord embed title for an event type
func (d *DiscordModule) getTitleForEvent(event NotificationEvent) string {
	switch event {
	case EventFailure:
		return "❌ Upload Failed"
	case EventSkip:
		return "⏭️ Upload Skipped"
	case EventComplete:
		return "✅ Upload Complete"
	default:
		return "📢 Notification"
	}
}
