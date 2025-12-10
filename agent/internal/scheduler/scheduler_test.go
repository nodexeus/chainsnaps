package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nodexeus/agent/internal/config"
	"github.com/nodexeus/agent/internal/database"
	"github.com/nodexeus/agent/internal/notification"
	"github.com/nodexeus/agent/internal/protocol"
	"github.com/nodexeus/agent/internal/upload"
	"github.com/sirupsen/logrus"
)

// Mock implementations for testing

type mockJob struct {
	runFunc  func(ctx context.Context) error
	runCount int
	mu       sync.Mutex
}

func (m *mockJob) Run(ctx context.Context) error {
	m.mu.Lock()
	m.runCount++
	m.mu.Unlock()

	if m.runFunc != nil {
		return m.runFunc(ctx)
	}
	return nil
}

func (m *mockJob) getRunCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runCount
}

type mockUploadManager struct {
	shouldSkipFunc      func(ctx context.Context, nodeName string) (bool, error)
	initiateUploadFunc  func(ctx context.Context, nodeName string, triggerType string) (int64, error)
	monitorProgressFunc func(ctx context.Context, uploadID int64, nodeName string) error
}

func (m *mockUploadManager) ShouldSkipUpload(ctx context.Context, nodeName string) (bool, error) {
	if m.shouldSkipFunc != nil {
		return m.shouldSkipFunc(ctx, nodeName)
	}
	return false, nil
}

func (m *mockUploadManager) InitiateUpload(ctx context.Context, nodeName string, triggerType string) (int64, error) {
	if m.initiateUploadFunc != nil {
		return m.initiateUploadFunc(ctx, nodeName, triggerType)
	}
	return 1, nil
}

func (m *mockUploadManager) MonitorUploadProgress(ctx context.Context, uploadID int64, nodeName string) error {
	if m.monitorProgressFunc != nil {
		return m.monitorProgressFunc(ctx, uploadID, nodeName)
	}
	return nil
}

func (m *mockUploadManager) CheckUploadStatus(ctx context.Context, nodeName string) (*upload.UploadStatus, error) {
	return &upload.UploadStatus{IsRunning: false}, nil
}

type mockDatabase struct {
	storeMetricsFunc      func(ctx context.Context, metrics database.NodeMetrics) error
	createUploadFunc      func(ctx context.Context, upload database.Upload) (int64, error)
	getRunningUploadsFunc func(ctx context.Context) ([]database.Upload, error)
}

func (m *mockDatabase) StoreNodeMetrics(ctx context.Context, metrics database.NodeMetrics) error {
	if m.storeMetricsFunc != nil {
		return m.storeMetricsFunc(ctx, metrics)
	}
	return nil
}

func (m *mockDatabase) CreateUpload(ctx context.Context, upload database.Upload) (int64, error) {
	if m.createUploadFunc != nil {
		return m.createUploadFunc(ctx, upload)
	}
	return 1, nil
}

func (m *mockDatabase) UpdateUpload(ctx context.Context, upload database.Upload) error {
	return nil
}

func (m *mockDatabase) GetRunningUploads(ctx context.Context) ([]database.Upload, error) {
	if m.getRunningUploadsFunc != nil {
		return m.getRunningUploadsFunc(ctx)
	}
	return []database.Upload{}, nil
}

func (m *mockDatabase) GetRunningUploadForNode(ctx context.Context, nodeName string) (*database.Upload, error) {
	return nil, nil
}

func (m *mockDatabase) StoreUploadProgress(ctx context.Context, progress database.UploadProgress) error {
	return nil
}

type mockProtocolModule struct {
	name               string
	collectMetricsFunc func(ctx context.Context, config config.NodeConfig) (map[string]interface{}, error)
}

func (m *mockProtocolModule) Name() string {
	return m.name
}

func (m *mockProtocolModule) CollectMetrics(ctx context.Context, cfg config.NodeConfig) (map[string]interface{}, error) {
	if m.collectMetricsFunc != nil {
		return m.collectMetricsFunc(ctx, cfg)
	}
	return map[string]interface{}{"test": "data"}, nil
}

type mockNotificationModule struct {
	name     string
	sendFunc func(ctx context.Context, url string, payload notification.NotificationPayload) error
}

func (m *mockNotificationModule) Name() string {
	return m.name
}

func (m *mockNotificationModule) Send(ctx context.Context, url string, payload notification.NotificationPayload) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, url, payload)
	}
	return nil
}

// Test CronScheduler

func TestCronScheduler_AddJob(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests

	scheduler := NewCronScheduler(logger)

	job := &mockJob{}

	// Test adding a valid job
	err := scheduler.AddJob("@every 1s", job)
	if err != nil {
		t.Fatalf("Failed to add job: %v", err)
	}

	// Test adding a job with invalid schedule
	err = scheduler.AddJob("invalid schedule", job)
	if err == nil {
		t.Fatal("Expected error for invalid schedule, got nil")
	}
}

func TestCronScheduler_StartStop(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	scheduler := NewCronScheduler(logger)

	executed := make(chan struct{}, 10)
	job := &mockJob{
		runFunc: func(ctx context.Context) error {
			executed <- struct{}{}
			return nil
		},
	}

	err := scheduler.AddJob("* * * * * *", job) // Every second
	if err != nil {
		t.Fatalf("Failed to add job: %v", err)
	}

	scheduler.Start()

	// Wait for at least one execution
	select {
	case <-executed:
		// Job executed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Job did not execute within timeout")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = scheduler.Stop(ctx)
	if err != nil {
		t.Fatalf("Failed to stop scheduler: %v", err)
	}

	runCount := job.getRunCount()
	if runCount < 1 {
		t.Errorf("Expected job to run at least 1 time, got %d", runCount)
	}
}

func TestCronScheduler_JobPanicRecovery(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	scheduler := NewCronScheduler(logger)

	executed := make(chan struct{}, 10)

	panicJob := &mockJob{
		runFunc: func(ctx context.Context) error {
			executed <- struct{}{}
			panic("test panic")
		},
	}

	err := scheduler.AddJob("* * * * * *", panicJob) // Every second
	if err != nil {
		t.Fatalf("Failed to add job: %v", err)
	}

	scheduler.Start()

	// Wait for at least one execution
	select {
	case <-executed:
		// Job executed and panicked
	case <-time.After(2 * time.Second):
		t.Fatal("Job did not execute within timeout")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Scheduler should handle panic and continue
	err = scheduler.Stop(ctx)
	if err != nil {
		t.Fatalf("Failed to stop scheduler after panic: %v", err)
	}
}

// Test NodeUploadJob

func TestNodeUploadJob_SkipWhenUploadRunning(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	uploadManager := &mockUploadManager{
		shouldSkipFunc: func(ctx context.Context, nodeName string) (bool, error) {
			return true, nil // Upload is running
		},
	}

	db := &mockDatabase{}
	protocolRegistry := protocol.NewRegistry()
	notifyRegistry := notification.NewRegistry()

	job := NewNodeUploadJob(
		"test-node",
		config.NodeConfig{Protocol: "ethereum"},
		protocolRegistry,
		uploadManager,
		db,
		notifyRegistry,
		nil,
		logger,
	)

	ctx := context.Background()
	err := job.Run(ctx)

	// Should not return error when skipping
	if err != nil {
		t.Errorf("Expected no error when skipping upload, got: %v", err)
	}
}

func TestNodeUploadJob_FullWorkflow(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	metricsStored := false
	uploadInitiated := false

	uploadManager := &mockUploadManager{
		shouldSkipFunc: func(ctx context.Context, nodeName string) (bool, error) {
			return false, nil // Upload not running
		},
		initiateUploadFunc: func(ctx context.Context, nodeName string, triggerType string) (int64, error) {
			uploadInitiated = true
			if triggerType != "scheduled" {
				t.Errorf("Expected trigger type 'scheduled', got '%s'", triggerType)
			}
			return 123, nil
		},
	}

	db := &mockDatabase{
		storeMetricsFunc: func(ctx context.Context, metrics database.NodeMetrics) error {
			metricsStored = true
			if metrics.NodeName != "test-node" {
				t.Errorf("Expected node name 'test-node', got '%s'", metrics.NodeName)
			}
			if metrics.Protocol != "ethereum" {
				t.Errorf("Expected protocol 'ethereum', got '%s'", metrics.Protocol)
			}
			return nil
		},
	}

	protocolRegistry := protocol.NewRegistry()
	mockProtocol := &mockProtocolModule{
		name: "ethereum",
		collectMetricsFunc: func(ctx context.Context, config config.NodeConfig) (map[string]interface{}, error) {
			return map[string]interface{}{"block": 12345}, nil
		},
	}
	protocolRegistry.Register(mockProtocol)

	notifyRegistry := notification.NewRegistry()

	job := NewNodeUploadJob(
		"test-node",
		config.NodeConfig{Protocol: "ethereum", Type: "archive"},
		protocolRegistry,
		uploadManager,
		db,
		notifyRegistry,
		nil,
		logger,
	)

	ctx := context.Background()
	err := job.Run(ctx)

	if err != nil {
		t.Fatalf("Job execution failed: %v", err)
	}

	if !metricsStored {
		t.Error("Expected metrics to be stored")
	}

	if !uploadInitiated {
		t.Error("Expected upload to be initiated")
	}
}

func TestNodeUploadJob_NodeIsolation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Simulate failure in protocol module
	uploadManager := &mockUploadManager{
		shouldSkipFunc: func(ctx context.Context, nodeName string) (bool, error) {
			return false, nil
		},
	}

	db := &mockDatabase{}

	protocolRegistry := protocol.NewRegistry()
	mockProtocol := &mockProtocolModule{
		name: "ethereum",
		collectMetricsFunc: func(ctx context.Context, config config.NodeConfig) (map[string]interface{}, error) {
			return nil, errors.New("protocol error")
		},
	}
	protocolRegistry.Register(mockProtocol)

	notifyRegistry := notification.NewRegistry()

	job := NewNodeUploadJob(
		"test-node",
		config.NodeConfig{Protocol: "ethereum"},
		protocolRegistry,
		uploadManager,
		db,
		notifyRegistry,
		nil,
		logger,
	)

	ctx := context.Background()
	err := job.Run(ctx)

	// Job should continue even with protocol error (stores partial metrics)
	// The error should not prevent other nodes from being processed
	if err != nil {
		t.Logf("Job returned error (expected for this test): %v", err)
	}
}

func TestNodeUploadJob_NotificationSending(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	notificationSent := false
	var sentEvent notification.NotificationEvent

	uploadManager := &mockUploadManager{
		shouldSkipFunc: func(ctx context.Context, nodeName string) (bool, error) {
			return true, nil // Skip to trigger skip notification
		},
	}

	db := &mockDatabase{}
	protocolRegistry := protocol.NewRegistry()
	notifyRegistry := notification.NewRegistry()

	mockNotify := &mockNotificationModule{
		name: "discord",
		sendFunc: func(ctx context.Context, url string, payload notification.NotificationPayload) error {
			notificationSent = true
			sentEvent = payload.Event
			return nil
		},
	}
	notifyRegistry.Register(mockNotify)

	notifyConfig := &config.NotificationConfig{
		Skip: true,
		Types: map[string]config.NotificationTypeConfig{
			"discord": {URL: "https://example.com/webhook"},
		},
	}

	job := NewNodeUploadJob(
		"test-node",
		config.NodeConfig{Protocol: "ethereum"},
		protocolRegistry,
		uploadManager,
		db,
		notifyRegistry,
		notifyConfig,
		logger,
	)

	ctx := context.Background()
	job.Run(ctx)

	if !notificationSent {
		t.Error("Expected notification to be sent")
	}

	if sentEvent != notification.EventSkip {
		t.Errorf("Expected EventSkip, got %v", sentEvent)
	}
}

// Test UploadMonitorJob

func TestUploadMonitorJob_NoRunningUploads(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	uploadManager := &mockUploadManager{}

	db := &mockDatabase{
		getRunningUploadsFunc: func(ctx context.Context) ([]database.Upload, error) {
			return []database.Upload{}, nil
		},
	}

	job := NewUploadMonitorJob(uploadManager, db, map[string]config.NodeConfig{}, logger)

	ctx := context.Background()
	err := job.Run(ctx)

	if err != nil {
		t.Errorf("Expected no error with no running uploads, got: %v", err)
	}
}

func TestUploadMonitorJob_MonitorsMultipleUploads(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	monitoredUploads := make(map[int64]bool)
	var mu sync.Mutex

	uploadManager := &mockUploadManager{
		monitorProgressFunc: func(ctx context.Context, uploadID int64, nodeName string) error {
			mu.Lock()
			monitoredUploads[uploadID] = true
			mu.Unlock()
			return nil
		},
	}

	db := &mockDatabase{
		getRunningUploadsFunc: func(ctx context.Context) ([]database.Upload, error) {
			return []database.Upload{
				{ID: 1, NodeName: "node1", Status: "running"},
				{ID: 2, NodeName: "node2", Status: "running"},
				{ID: 3, NodeName: "node3", Status: "running"},
			}, nil
		},
	}

	job := NewUploadMonitorJob(uploadManager, db, map[string]config.NodeConfig{}, logger)

	ctx := context.Background()
	err := job.Run(ctx)

	if err != nil {
		t.Fatalf("Job execution failed: %v", err)
	}

	// Check that all uploads were monitored
	mu.Lock()
	defer mu.Unlock()

	if len(monitoredUploads) != 3 {
		t.Errorf("Expected 3 uploads to be monitored, got %d", len(monitoredUploads))
	}

	for _, id := range []int64{1, 2, 3} {
		if !monitoredUploads[id] {
			t.Errorf("Upload %d was not monitored", id)
		}
	}
}

func TestUploadMonitorJob_NodeIsolation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	monitoredUploads := make(map[int64]bool)
	var mu sync.Mutex

	uploadManager := &mockUploadManager{
		monitorProgressFunc: func(ctx context.Context, uploadID int64, nodeName string) error {
			mu.Lock()
			monitoredUploads[uploadID] = true
			mu.Unlock()

			// Simulate failure for upload 2
			if uploadID == 2 {
				return errors.New("monitoring failed")
			}
			return nil
		},
	}

	db := &mockDatabase{
		getRunningUploadsFunc: func(ctx context.Context) ([]database.Upload, error) {
			return []database.Upload{
				{ID: 1, NodeName: "node1", Status: "running"},
				{ID: 2, NodeName: "node2", Status: "running"},
				{ID: 3, NodeName: "node3", Status: "running"},
			}, nil
		},
	}

	job := NewUploadMonitorJob(uploadManager, db, map[string]config.NodeConfig{}, logger)

	ctx := context.Background()
	err := job.Run(ctx)

	// Job should not return error even if one upload fails (node isolation)
	if err != nil {
		t.Errorf("Expected no error with node isolation, got: %v", err)
	}

	// All uploads should still be attempted
	mu.Lock()
	defer mu.Unlock()

	if len(monitoredUploads) != 3 {
		t.Errorf("Expected 3 uploads to be monitored, got %d", len(monitoredUploads))
	}
}
