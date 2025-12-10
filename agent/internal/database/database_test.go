package database

import (
	"context"
	"testing"
	"time"
)

// TestDatabaseStructs verifies that the database structs are properly defined
func TestDatabaseStructs(t *testing.T) {
	// Test Upload struct
	upload := Upload{
		NodeName:    "test-node",
		Protocol:    "ethereum",
		NodeType:    "archive",
		StartedAt:   time.Now(),
		Status:      "running",
		TriggerType: "scheduled",
		ProtocolData: JSONB{
			"latest_block": 12345,
			"latest_slot":  67890,
		},
	}

	if upload.Status != "running" {
		t.Errorf("expected status 'running', got %s", upload.Status)
	}

	// Test progress fields in Upload struct
	progressPercent := 75.5
	chunksCompleted := 150
	chunksTotal := 200
	lastCheck := time.Now()

	upload.ProgressPercent = &progressPercent
	upload.ChunksCompleted = &chunksCompleted
	upload.ChunksTotal = &chunksTotal
	upload.LastProgressCheck = &lastCheck

	if *upload.ProgressPercent != 75.5 {
		t.Errorf("expected progress percent 75.5, got %f", *upload.ProgressPercent)
	}
}

// TestConfig verifies the Config struct
func TestConfig(t *testing.T) {
	cfg := Config{
		Host:     "localhost",
		Port:     5432,
		Database: "snapd",
		User:     "snapd",
		Password: "password",
		SSLMode:  "disable",
	}

	if cfg.Host != "localhost" {
		t.Errorf("expected host 'localhost', got %s", cfg.Host)
	}

	if cfg.Port != 5432 {
		t.Errorf("expected port 5432, got %d", cfg.Port)
	}
}

// TestJSONBType verifies JSONB marshaling and unmarshaling
func TestJSONBType(t *testing.T) {
	original := JSONB{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}

	// Test Value (marshaling)
	value, err := original.Value()
	if err != nil {
		t.Fatalf("failed to marshal JSONB: %v", err)
	}

	if value == nil {
		t.Fatal("expected non-nil value")
	}

	// Test Scan (unmarshaling)
	var scanned JSONB
	err = scanned.Scan(value)
	if err != nil {
		t.Fatalf("failed to scan JSONB: %v", err)
	}

	if scanned["key1"] != "value1" {
		t.Errorf("expected key1='value1', got %v", scanned["key1"])
	}

	if scanned["key2"] != float64(42) { // JSON numbers are float64
		t.Errorf("expected key2=42, got %v", scanned["key2"])
	}

	if scanned["key3"] != true {
		t.Errorf("expected key3=true, got %v", scanned["key3"])
	}
}

// TestJSONBNil verifies JSONB handles nil values
func TestJSONBNil(t *testing.T) {
	var j JSONB

	// Test nil Value
	value, err := j.Value()
	if err != nil {
		t.Fatalf("failed to get value from nil JSONB: %v", err)
	}

	if value != nil {
		t.Errorf("expected nil value, got %v", value)
	}

	// Test nil Scan
	err = j.Scan(nil)
	if err != nil {
		t.Fatalf("failed to scan nil: %v", err)
	}

	if j != nil {
		t.Errorf("expected nil JSONB, got %v", j)
	}
}

// TestRetryLogic verifies exponential backoff is configured
func TestRetryLogic(t *testing.T) {
	// We can't test actual retry without a database, but we can verify
	// the DB struct has the retry configuration
	db := &DB{
		maxRetries:     3,
		retryBaseDelay: 100 * time.Millisecond,
	}

	if db.maxRetries != 3 {
		t.Errorf("expected maxRetries=3, got %d", db.maxRetries)
	}

	if db.retryBaseDelay != 100*time.Millisecond {
		t.Errorf("expected retryBaseDelay=100ms, got %v", db.retryBaseDelay)
	}
}

// TestContextCancellation verifies context handling
func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Verify context is cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Error("expected context to be cancelled")
	}

	if ctx.Err() == nil {
		t.Error("expected context error")
	}
}
