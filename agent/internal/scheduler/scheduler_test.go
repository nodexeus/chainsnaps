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
	shouldSkipFunc                     func(ctx context.Context, nodeName string) (bool, error)
	initiateUploadFunc                 func(ctx context.Context, nodeName string, triggerType string) (int64, error)
	initiateUploadWithProtocolDataFunc func(ctx context.Context, nodeName string, triggerType string, protocol string, nodeType string, protocolData map[string]interface{}) (int64, error)
	monitorProgressFunc                func(ctx context.Context, uploadID int64, nodeName string) error
	checkUploadStatusFunc              func(ctx context.Context, nodeName string) (*upload.UploadStatus, error)
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

func (m *mockUploadManager) InitiateUploadWithProtocolData(ctx context.Context, nodeName string, triggerType string, protocol string, nodeType string, protocolData map[string]interface{}) (int64, error) {
	if m.initiateUploadWithProtocolDataFunc != nil {
		return m.initiateUploadWithProtocolDataFunc(ctx, nodeName, triggerType, protocol, nodeType, protocolData)
	}
	// Fallback to regular InitiateUpload method
	return m.InitiateUpload(ctx, nodeName, triggerType)
}

func (m *mockUploadManager) MonitorUploadProgress(ctx context.Context, uploadID int64, nodeName string) error {
	if m.monitorProgressFunc != nil {
		return m.monitorProgressFunc(ctx, uploadID, nodeName)
	}
	return nil
}

func (m *mockUploadManager) CheckUploadStatus(ctx context.Context, nodeName string) (*upload.UploadStatus, error) {
	if m.checkUploadStatusFunc != nil {
		return m.checkUploadStatusFunc(ctx, nodeName)
	}
	return &upload.UploadStatus{IsRunning: false}, nil
}

type mockDatabase struct {
	createUploadFunc      func(ctx context.Context, upload database.Upload) (int64, error)
	getRunningUploadsFunc func(ctx context.Context) ([]database.Upload, error)
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

func (m *mockDatabase) UpsertRunningUpload(ctx context.Context, upload database.Upload) (int64, error) {
	// For tests, just return a mock ID
	return 123, nil
}

func (m *mockDatabase) GetLatestCompletedUploadForNode(ctx context.Context, nodeName string) (*database.Upload, error) {
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

	// Declare db first so it can be used in uploadManager closure
	db := &mockDatabase{
		createUploadFunc: func(ctx context.Context, upload database.Upload) (int64, error) {
			metricsStored = true
			if upload.NodeName != "test-node" {
				t.Errorf("Expected node name 'test-node', got '%s'", upload.NodeName)
			}
			if upload.Protocol != "ethereum" {
				t.Errorf("Expected protocol 'ethereum', got '%s'", upload.Protocol)
			}
			return 1, nil
		},
	}

	uploadManager := &mockUploadManager{
		shouldSkipFunc: func(ctx context.Context, nodeName string) (bool, error) {
			return false, nil // Upload not running
		},
		initiateUploadWithProtocolDataFunc: func(ctx context.Context, nodeName string, triggerType string, protocol string, nodeType string, protocolData map[string]interface{}) (int64, error) {
			uploadInitiated = true
			if triggerType != "scheduled" {
				t.Errorf("Expected trigger type 'scheduled', got '%s'", triggerType)
			}
			if protocol != "ethereum" {
				t.Errorf("Expected protocol 'ethereum', got '%s'", protocol)
			}
			// Simulate what the real upload manager does - call CreateUpload on the database
			upload := database.Upload{
				NodeName:     nodeName,
				Protocol:     protocol,
				NodeType:     "archive", // Mock node type
				StartedAt:    time.Now(),
				Status:       "running",
				TriggerType:  triggerType,
				ProtocolData: database.JSONB(protocolData),
			}
			return db.CreateUpload(ctx, upload)
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
func TestUploadMonitorJob_ExternalUploadDiscovery(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	var createdUploads []database.Upload
	var mu sync.Mutex

	// Mock upload manager that reports a running upload for "external-node"
	uploadManager := &mockUploadManager{
		checkUploadStatusFunc: func(ctx context.Context, nodeName string) (*upload.UploadStatus, error) {
			if nodeName == "external-node" {
				return &upload.UploadStatus{
					IsRunning: true,
					Progress: upload.JSONB{
						"status":   "running",
						"progress": "50.0%",
					},
				}, nil
			}
			return &upload.UploadStatus{IsRunning: false}, nil
		},
		monitorProgressFunc: func(ctx context.Context, uploadID int64, nodeName string) error {
			return nil
		},
	}

	// Mock database that tracks created uploads
	db := &mockDatabase{
		getRunningUploadsFunc: func(ctx context.Context) ([]database.Upload, error) {
			mu.Lock()
			defer mu.Unlock()
			// Return existing tracked uploads (none initially)
			return createdUploads, nil
		},
		createUploadFunc: func(ctx context.Context, upload database.Upload) (int64, error) {
			mu.Lock()
			defer mu.Unlock()
			upload.ID = int64(len(createdUploads) + 1)
			createdUploads = append(createdUploads, upload)
			return upload.ID, nil
		},
	}

	// Configure nodes - one with external upload, one without
	nodeConfigs := map[string]config.NodeConfig{
		"external-node": {Protocol: "ethereum", Type: "execution"},
		"normal-node":   {Protocol: "ethereum", Type: "execution"},
	}

	job := NewUploadMonitorJob(uploadManager, db, nodeConfigs, logger)

	ctx := context.Background()

	// First run - should discover external upload
	err := job.Run(ctx)
	if err != nil {
		t.Errorf("Expected no error on first run, got: %v", err)
	}

	mu.Lock()
	if len(createdUploads) != 1 {
		t.Errorf("Expected 1 external upload to be created, got %d", len(createdUploads))
	}
	if len(createdUploads) > 0 {
		upload := createdUploads[0]
		if upload.NodeName != "external-node" {
			t.Errorf("Expected external upload for 'external-node', got '%s'", upload.NodeName)
		}
		if upload.TriggerType != "external" {
			t.Errorf("Expected trigger_type 'external', got '%s'", upload.TriggerType)
		}
	}
	mu.Unlock()

	// Second run - should NOT create duplicate upload
	err = job.Run(ctx)
	if err != nil {
		t.Errorf("Expected no error on second run, got: %v", err)
	}

	mu.Lock()
	if len(createdUploads) != 1 {
		t.Errorf("Expected still only 1 upload after second run, got %d", len(createdUploads))
	}
	mu.Unlock()
}

func TestUploadMonitorJob_DoesNotDuplicateTrackedUploads(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	var createdUploads []database.Upload
	var mu sync.Mutex

	// Mock upload manager that reports running upload
	uploadManager := &mockUploadManager{
		checkUploadStatusFunc: func(ctx context.Context, nodeName string) (*upload.UploadStatus, error) {
			return &upload.UploadStatus{
				IsRunning: true,
				Progress: upload.JSONB{
					"status":   "running",
					"progress": "75.0%",
				},
			}, nil
		},
		monitorProgressFunc: func(ctx context.Context, uploadID int64, nodeName string) error {
			return nil
		},
	}

	// Mock database with existing tracked upload
	existingUpload := database.Upload{
		ID:          83,
		NodeName:    "tracked-node",
		Protocol:    "ethereum",
		NodeType:    "execution",
		Status:      "running",
		TriggerType: "scheduled",
		StartedAt:   time.Now().Add(-1 * time.Hour),
	}

	db := &mockDatabase{
		getRunningUploadsFunc: func(ctx context.Context) ([]database.Upload, error) {
			return []database.Upload{existingUpload}, nil
		},
		createUploadFunc: func(ctx context.Context, upload database.Upload) (int64, error) {
			mu.Lock()
			defer mu.Unlock()
			upload.ID = int64(len(createdUploads) + 100)
			createdUploads = append(createdUploads, upload)
			return upload.ID, nil
		},
	}

	// Configure node that already has tracked upload
	nodeConfigs := map[string]config.NodeConfig{
		"tracked-node": {Protocol: "ethereum", Type: "execution"},
	}

	job := NewUploadMonitorJob(uploadManager, db, nodeConfigs, logger)

	ctx := context.Background()
	err := job.Run(ctx)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	mu.Lock()
	if len(createdUploads) != 0 {
		t.Errorf("Expected no new uploads to be created for already tracked node, got %d", len(createdUploads))
	}
	mu.Unlock()
}
