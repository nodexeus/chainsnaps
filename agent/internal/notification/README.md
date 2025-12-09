# Notification Module System

The notification module system provides a pluggable architecture for sending notifications to external services when specific events occur during snapshot daemon operations.

## Architecture

The notification system consists of:

1. **NotificationModule Interface**: Defines the contract that all notification modules must implement
2. **Registry**: Manages registration and retrieval of notification modules
3. **NotificationPayload**: Standard structure for event data
4. **Concrete Implementations**: Specific notification modules (e.g., Discord)

## NotificationModule Interface

```go
type NotificationModule interface {
    Name() string
    Send(ctx context.Context, url string, payload NotificationPayload) error
}
```

## Event Types

The system supports three event types:

- `EventFailure`: Triggered when an upload operation fails
- `EventSkip`: Triggered when an upload is skipped (already running)
- `EventComplete`: Triggered when an upload completes successfully

## Usage

### Registering Modules

```go
registry := notification.NewRegistry()

// Register Discord module
discordModule := notification.NewDiscordModule()
if err := registry.Register(discordModule); err != nil {
    log.Fatal(err)
}
```

### Sending Notifications

```go
// Get the notification module
module, err := registry.Get("discord")
if err != nil {
    log.Fatal(err)
}

// Create payload
payload := notification.NotificationPayload{
    Event:     notification.EventComplete,
    NodeName:  "ethereum-mainnet",
    Timestamp: time.Now(),
    Message:   "Upload completed successfully",
    Details: map[string]interface{}{
        "duration": "2h30m",
        "size":     "500GB",
    },
}

// Send notification
ctx := context.Background()
err = module.Send(ctx, "https://discord.com/api/webhooks/...", payload)
```

## Implementing New Notification Modules

To add support for a new notification service:

1. Create a new file (e.g., `slack.go`)
2. Implement the `NotificationModule` interface
3. Register the module in the registry during daemon initialization

Example:

```go
type SlackModule struct{}

func (s *SlackModule) Name() string {
    return "slack"
}

func (s *SlackModule) Send(ctx context.Context, url string, payload NotificationPayload) error {
    // Implement Slack-specific webhook logic
    return nil
}
```

## Discord Module

The Discord module formats notifications as rich embeds with:

- Color-coded based on event type (red for failures, orange for skips, green for completions)
- Embedded fields for node name, event type, and timestamp
- Additional detail fields from the payload
- Emoji icons in titles for visual clarity

### Discord Webhook Setup

1. Go to your Discord server settings
2. Navigate to Integrations → Webhooks
3. Create a new webhook and copy the URL
4. Add the URL to your daemon configuration

## Future Enhancements

### Multiple Notification Types Per Node

**Current Limitation**: The current implementation supports only one notification type per node (either global or per-node override).

**Proposed Enhancement**: Support multiple notification types simultaneously, allowing notifications to be sent to multiple services (e.g., both Discord and Slack) for the same events.

**Example Future Configuration**:
```yaml
notifications:
  - type: discord
    url: https://discord.com/api/webhooks/...
    failure: true
    skip: false
    complete: true
  - type: slack
    url: https://hooks.slack.com/services/...
    failure: true
    skip: true
    complete: false
```

**Implementation Considerations**:
- Change `NotificationConfig` from a single struct to a slice of notification configs
- Update validation logic to handle multiple notification types
- Modify notification sending logic to iterate through all configured types
- Ensure failures in one notification type don't block others
- Add per-type event filtering (failure/skip/complete flags per notification type)

**Benefits**:
- Send critical failures to multiple channels for redundancy
- Route different event types to different services (e.g., failures to PagerDuty, completions to Slack)
- Support team-specific notification preferences without code changes

This enhancement would require updates to:
1. Requirements document (Requirement 12)
2. Design document (NotificationConfig structure)
3. Config package (validation and parsing)
4. Notification sending logic in scheduler/orchestration
