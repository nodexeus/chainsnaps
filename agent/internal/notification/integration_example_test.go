package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/nodexeus/agent/internal/config"
)

// Example_integration demonstrates the complete notification system integration
func Example_integration() {
	// Step 1: Create and configure the notification registry
	registry := NewRegistry()

	// Step 2: Register notification modules
	discordModule := NewDiscordModule()
	if err := registry.Register(discordModule); err != nil {
		fmt.Printf("Failed to register Discord module: %v\n", err)
		return
	}

	// Step 3: Set up config validation
	config.SetNotificationValidator(registry)

	// Step 4: Verify module registration
	if !registry.IsRegistered("discord") {
		fmt.Println("Discord module not registered")
		return
	}

	// Step 5: Create notification payloads for different events
	ctx := context.Background()
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Failure event
	failurePayload := NotificationPayload{
		Event:     EventFailure,
		NodeName:  "ethereum-mainnet",
		Timestamp: now,
		Message:   "Upload failed due to network error",
		Details: map[string]interface{}{
			"error": "connection timeout",
		},
	}

	// Skip event
	skipPayload := NotificationPayload{
		Event:     EventSkip,
		NodeName:  "arbitrum-one",
		Timestamp: now,
		Message:   "Upload skipped - already running",
		Details: map[string]interface{}{
			"existing_upload_id": "12345",
		},
	}

	// Complete event
	completePayload := NotificationPayload{
		Event:     EventComplete,
		NodeName:  "ethereum-mainnet",
		Timestamp: now,
		Message:   "Upload completed successfully",
		Details: map[string]interface{}{
			"duration": "2h30m",
			"size":     "500GB",
		},
	}

	// Step 6: Retrieve module and send notifications
	module, err := registry.Get("discord")
	if err != nil {
		fmt.Printf("Failed to get module: %v\n", err)
		return
	}

	// In a real scenario, these would be sent to actual webhook URLs
	// For this example, we just demonstrate the API
	fmt.Printf("Module ready: %s\n", module.Name())
	fmt.Printf("Failure event: %s\n", failurePayload.Event)
	fmt.Printf("Skip event: %s\n", skipPayload.Event)
	fmt.Printf("Complete event: %s\n", completePayload.Event)

	// Demonstrate sending (would fail without real webhook)
	_ = module.Send(ctx, "https://example.com/webhook", failurePayload)

	// Output:
	// Module ready: discord
	// Failure event: failure
	// Skip event: skip
	// Complete event: complete
}
