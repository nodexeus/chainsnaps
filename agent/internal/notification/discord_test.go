package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDiscordModule_Name(t *testing.T) {
	module := NewDiscordModule()
	if module.Name() != "discord" {
		t.Errorf("Name() = %v, want 'discord'", module.Name())
	}
}

func TestDiscordModule_Send(t *testing.T) {
	tests := []struct {
		name           string
		payload        NotificationPayload
		serverStatus   int
		wantErr        bool
		validateFields func(t *testing.T, body map[string]interface{})
	}{
		{
			name: "successful send with complete event",
			payload: NotificationPayload{
				Event:     EventComplete,
				NodeName:  "test-node",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Message:   "Upload completed",
				Details: map[string]interface{}{
					"duration": "2h",
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			validateFields: func(t *testing.T, body map[string]interface{}) {
				embeds, ok := body["embeds"].([]interface{})
				if !ok || len(embeds) == 0 {
					t.Fatal("embeds not found or empty")
				}
				embed := embeds[0].(map[string]interface{})

				if embed["title"] != "✅ Upload Complete" {
					t.Errorf("title = %v, want '✅ Upload Complete'", embed["title"])
				}
				if embed["description"] != "Upload completed" {
					t.Errorf("description = %v, want 'Upload completed'", embed["description"])
				}
				if embed["color"] != float64(0x00FF00) {
					t.Errorf("color = %v, want %v", embed["color"], 0x00FF00)
				}
			},
		},
		{
			name: "successful send with failure event",
			payload: NotificationPayload{
				Event:     EventFailure,
				NodeName:  "test-node",
				Timestamp: time.Now(),
				Message:   "Upload failed",
				Details:   map[string]interface{}{},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			validateFields: func(t *testing.T, body map[string]interface{}) {
				embeds := body["embeds"].([]interface{})
				embed := embeds[0].(map[string]interface{})

				if embed["title"] != "❌ Upload Failed" {
					t.Errorf("title = %v, want '❌ Upload Failed'", embed["title"])
				}
				if embed["color"] != float64(0xFF0000) {
					t.Errorf("color = %v, want %v", embed["color"], 0xFF0000)
				}
			},
		},
		{
			name: "successful send with skip event",
			payload: NotificationPayload{
				Event:     EventSkip,
				NodeName:  "test-node",
				Timestamp: time.Now(),
				Message:   "Upload skipped",
				Details:   map[string]interface{}{},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			validateFields: func(t *testing.T, body map[string]interface{}) {
				embeds := body["embeds"].([]interface{})
				embed := embeds[0].(map[string]interface{})

				if embed["title"] != "⏭️ Upload Skipped" {
					t.Errorf("title = %v, want '⏭️ Upload Skipped'", embed["title"])
				}
				if embed["color"] != float64(0xFFA500) {
					t.Errorf("color = %v, want %v", embed["color"], 0xFFA500)
				}
			},
		},
		{
			name: "server error",
			payload: NotificationPayload{
				Event:     EventComplete,
				NodeName:  "test-node",
				Timestamp: time.Now(),
				Message:   "Test",
				Details:   map[string]interface{}{},
			},
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
		},
		{
			name: "with multiple details",
			payload: NotificationPayload{
				Event:     EventComplete,
				NodeName:  "test-node",
				Timestamp: time.Now(),
				Message:   "Upload completed",
				Details: map[string]interface{}{
					"duration": "2h30m",
					"size":     "500GB",
					"blocks":   12345,
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			validateFields: func(t *testing.T, body map[string]interface{}) {
				embeds := body["embeds"].([]interface{})
				embed := embeds[0].(map[string]interface{})
				fields := embed["fields"].([]interface{})

				// Should have at least 3 base fields (Node, Event, Timestamp) + 3 detail fields
				if len(fields) < 6 {
					t.Errorf("fields length = %d, want at least 6", len(fields))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			var receivedBody map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and content type
				if r.Method != "POST" {
					t.Errorf("Method = %v, want POST", r.Method)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Content-Type = %v, want application/json", r.Header.Get("Content-Type"))
				}

				// Decode body
				if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
					t.Fatalf("Failed to decode request body: %v", err)
				}

				w.WriteHeader(tt.serverStatus)
			}))
			defer server.Close()

			// Send notification
			module := NewDiscordModule()
			ctx := context.Background()
			err := module.Send(ctx, server.URL, tt.payload)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Send() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Validate fields if provided
			if !tt.wantErr && tt.validateFields != nil {
				tt.validateFields(t, receivedBody)
			}
		})
	}
}

func TestDiscordModule_Send_ContextCancellation(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	module := NewDiscordModule()
	payload := NotificationPayload{
		Event:     EventComplete,
		NodeName:  "test-node",
		Timestamp: time.Now(),
		Message:   "Test",
		Details:   map[string]interface{}{},
	}

	err := module.Send(ctx, server.URL, payload)
	if err == nil {
		t.Error("Send() should fail with cancelled context")
	}
}

func TestDiscordModule_formatWebhookPayload(t *testing.T) {
	module := NewDiscordModule()
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	payload := NotificationPayload{
		Event:     EventComplete,
		NodeName:  "test-node",
		Timestamp: now,
		Message:   "Test message",
		Details: map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		},
	}

	result := module.formatWebhookPayload(payload)

	// Check embeds exist
	embeds, ok := result["embeds"].([]map[string]interface{})
	if !ok {
		t.Fatal("embeds not found or wrong type")
	}
	if len(embeds) != 1 {
		t.Fatalf("embeds length = %d, want 1", len(embeds))
	}

	embed := embeds[0]

	// Check embed fields
	if embed["title"] != "✅ Upload Complete" {
		t.Errorf("title = %v, want '✅ Upload Complete'", embed["title"])
	}
	if embed["description"] != "Test message" {
		t.Errorf("description = %v, want 'Test message'", embed["description"])
	}
	if embed["color"] != 0x00FF00 {
		t.Errorf("color = %v, want %v", embed["color"], 0x00FF00)
	}
	if embed["timestamp"] != now.Format(time.RFC3339) {
		t.Errorf("timestamp = %v, want %v", embed["timestamp"], now.Format(time.RFC3339))
	}

	// Check fields array
	fields, ok := embed["fields"].([]map[string]interface{})
	if !ok {
		t.Fatal("fields not found or wrong type")
	}

	// Should have Node, Event, Timestamp + 2 detail fields
	if len(fields) < 5 {
		t.Errorf("fields length = %d, want at least 5", len(fields))
	}

	// Verify Node field
	if fields[0]["name"] != "Node" || fields[0]["value"] != "test-node" {
		t.Errorf("Node field incorrect: %v", fields[0])
	}

	// Verify Event field
	if fields[1]["name"] != "Event" || fields[1]["value"] != "complete" {
		t.Errorf("Event field incorrect: %v", fields[1])
	}
}

func TestDiscordModule_getColorForEvent(t *testing.T) {
	module := NewDiscordModule()

	tests := []struct {
		event NotificationEvent
		want  int
	}{
		{EventFailure, 0xFF0000},
		{EventSkip, 0xFFA500},
		{EventComplete, 0x00FF00},
		{NotificationEvent("unknown"), 0x808080},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			got := module.getColorForEvent(tt.event)
			if got != tt.want {
				t.Errorf("getColorForEvent(%v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestDiscordModule_getTitleForEvent(t *testing.T) {
	module := NewDiscordModule()

	tests := []struct {
		event NotificationEvent
		want  string
	}{
		{EventFailure, "❌ Upload Failed"},
		{EventSkip, "⏭️ Upload Skipped"},
		{EventComplete, "✅ Upload Complete"},
		{NotificationEvent("unknown"), "📢 Notification"},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			got := module.getTitleForEvent(tt.event)
			if got != tt.want {
				t.Errorf("getTitleForEvent(%v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}
