package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/yourusername/snapd/internal/config"
)

// Example demonstrates how to use the notification module system
func Example() {
	// Create a new notification registry
	registry := NewRegistry()

	// Register notification modules
	registry.Register(NewDiscordModule())

	// Set up the config validator
	config.SetNotificationValidator(registry)

	// Check if a notification type is registered
	if registry.IsRegistered("discord") {
		fmt.Println("Discord notification is registered")
	}

	// Get a notification module
	discordModule, err := registry.Get("discord")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Retrieved module: %s\n", discordModule.Name())

	// Example of sending a notification (would need a real webhook URL)
	ctx := context.Background()
	payload := NotificationPayload{
		Event:     EventComplete,
		NodeName:  "ethereum-mainnet",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Message:   "Upload completed successfully",
		Details: map[string]interface{}{
			"duration": "2h30m",
		},
	}

	// Note: This would fail without a real webhook URL
	err = discordModule.Send(ctx, "https://discord.com/api/webhooks/invalid", payload)
	if err != nil {
		fmt.Printf("Notification would require a real webhook URL\n")
	}

	// Output:
	// Discord notification is registered
	// Retrieved module: discord
	// Notification would require a real webhook URL
}
