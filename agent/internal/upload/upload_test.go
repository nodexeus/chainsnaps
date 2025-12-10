package upload

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
)

// Mock implementations for testing

type mockExecutor struct {
	executeFunc func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error)
}

func (m *mockExecutor) Execute(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, command, args...)
	}
	return "", "", nil
}

type mockDatabase struct {
	createUploadFunc            func(ctx context.Context, upload Upload) (int64, error)
	updateUploadFunc            func(ctx context.Context, upload Upload) error
	getRunningUploadForNodeFunc func(ctx context.Context, nodeName string) (*Upload, error)
	storeUploadProgressFunc     func(ctx context.Context, progress UploadProgress) error
}

func (m *mockDatabase) CreateUpload(ctx context.Context, upload Upload) (int64, error) {
	if m.createUploadFunc != nil {
		return m.createUploadFunc(ctx, upload)
	}
	return 1, nil
}

func (m *mockDatabase) UpdateUpload(ctx context.Context, upload Upload) error {
	if m.updateUploadFunc != nil {
		return m.updateUploadFunc(ctx, upload)
	}
	return nil
}

func (m *mockDatabase) GetRunningUploadForNode(ctx context.Context, nodeName string) (*Upload, error) {
	if m.getRunningUploadForNodeFunc != nil {
		return m.getRunningUploadForNodeFunc(ctx, nodeName)
	}
	return nil, nil
}

func (m *mockDatabase) StoreUploadProgress(ctx context.Context, progress UploadProgress) error {
	if m.storeUploadProgressFunc != nil {
		return m.storeUploadProgressFunc(ctx, progress)
	}
	return nil
}

func TestCheckUploadStatus_BVOutput(t *testing.T) {
	tests := []struct {
		name            string
		output          string
		expectedRunning bool
		expectError     bool
	}{
		{
			name: "Upload completed",
			output: `status:           2025-12-07 13:41:43 UTC| Finished with exit code 0 and message 'Multi-client upload completed successfully'
progress:         100.00% (3248/3248 multi-client upload completed)
restart_count:    0
upgrade_blocking: true
logs:             <empty>`,
			expectedRunning: false,
			expectError:     false,
		},
		{
			name: "Upload running - actual format",
			output: `status:           2025-12-09 18:08:56 UTC| Running
progress:         0.18% (6/3252 multi-client upload (in progress clients))
restart_count:    0
upgrade_blocking: true
logs:             <empty>`,
			expectedRunning: true,
			expectError:     false,
		},
		{
			name: "Upload running - simple format",
			output: `status:           Running
progress:         75.50% (3100/3248 multi-client upload in progress)
restart_count:    0
upgrade_blocking: true
logs:             <empty>`,
			expectedRunning: true,
			expectError:     false,
		},
		{
			name: "Upload running with timestamp",
			output: `status:           2025-12-07 14:30:00 UTC| Running
progress:         50.00% (1624/3248 uploading)
restart_count:    1
upgrade_blocking: true
logs:             Some log output`,
			expectedRunning: true,
			expectError:     false,
		},
		{
			name: "Upload failed",
			output: `status:           2025-12-07 15:00:00 UTC| Finished with exit code 1 and message 'Upload failed'
progress:         45.00% (1461/3248 failed)
restart_count:    2
upgrade_blocking: true
logs:             Error details here`,
			expectedRunning: false,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockExecutor{
				executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
					return tt.output, "", nil
				},
			}

			manager := NewManager(executor, &mockDatabase{}, logrus.New())
			status, err := manager.CheckUploadStatus(context.Background(), "test-node")

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectError && status.IsRunning != tt.expectedRunning {
				t.Errorf("Expected IsRunning=%v, got %v", tt.expectedRunning, status.IsRunning)
			}
		})
	}
}

func TestCheckUploadStatus_EdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		output          string
		expectedRunning bool
	}{
		{
			name:            "No job found",
			output:          "No job found for upload",
			expectedRunning: false,
		},
		{
			name:            "Empty output",
			output:          "",
			expectedRunning: false,
		},
		{
			name:            "No upload message",
			output:          "No upload currently running",
			expectedRunning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockExecutor{
				executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
					return tt.output, "", nil
				},
			}

			manager := NewManager(executor, &mockDatabase{}, logrus.New())
			status, err := manager.CheckUploadStatus(context.Background(), "test-node")

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if status.IsRunning != tt.expectedRunning {
				t.Errorf("Expected IsRunning=%v, got %v", tt.expectedRunning, status.IsRunning)
			}
		})
	}
}

func TestCheckUploadStatus_CommandConstruction(t *testing.T) {
	var capturedCommand string
	var capturedArgs []string

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			capturedCommand = command
			capturedArgs = args
			return `{"running": false}`, "", nil
		},
	}

	manager := NewManager(executor, &mockDatabase{}, logrus.New())
	_, err := manager.CheckUploadStatus(context.Background(), "ethereum-mainnet")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedCommand := "bv"
	expectedArgs := []string{"node", "job", "ethereum-mainnet", "info", "upload"}

	if capturedCommand != expectedCommand {
		t.Errorf("Expected command %q, got %q", expectedCommand, capturedCommand)
	}

	if len(capturedArgs) != len(expectedArgs) {
		t.Fatalf("Expected %d args, got %d", len(expectedArgs), len(capturedArgs))
	}

	for i, arg := range expectedArgs {
		if capturedArgs[i] != arg {
			t.Errorf("Expected arg[%d]=%q, got %q", i, arg, capturedArgs[i])
		}
	}
}

func TestInitiateUpload_CommandConstruction(t *testing.T) {
	var capturedCommand string
	var capturedArgs []string

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			capturedCommand = command
			capturedArgs = args
			return "Upload started", "", nil
		},
	}

	db := &mockDatabase{
		createUploadFunc: func(ctx context.Context, upload Upload) (int64, error) {
			return 123, nil
		},
	}

	manager := NewManager(executor, db, logrus.New())
	_, err := manager.InitiateUpload(context.Background(), "arbitrum-one", "scheduled")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedCommand := "bv"
	expectedArgs := []string{"node", "run", "upload", "arbitrum-one"}

	if capturedCommand != expectedCommand {
		t.Errorf("Expected command %q, got %q", expectedCommand, capturedCommand)
	}

	if len(capturedArgs) != len(expectedArgs) {
		t.Fatalf("Expected %d args, got %d", len(expectedArgs), len(capturedArgs))
	}

	for i, arg := range expectedArgs {
		if capturedArgs[i] != arg {
			t.Errorf("Expected arg[%d]=%q, got %q", i, arg, capturedArgs[i])
		}
	}
}

func TestInitiateUpload_DatabasePersistence(t *testing.T) {
	var capturedUpload Upload

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			return "Upload started", "", nil
		},
	}

	db := &mockDatabase{
		createUploadFunc: func(ctx context.Context, upload Upload) (int64, error) {
			capturedUpload = upload
			return 456, nil
		},
	}

	manager := NewManager(executor, db, logrus.New())
	uploadID, err := manager.InitiateUpload(context.Background(), "test-node", "manual")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if uploadID != 456 {
		t.Errorf("Expected upload ID 456, got %d", uploadID)
	}

	if capturedUpload.NodeName != "test-node" {
		t.Errorf("Expected node name 'test-node', got %q", capturedUpload.NodeName)
	}

	if capturedUpload.Status != "running" {
		t.Errorf("Expected status 'running', got %q", capturedUpload.Status)
	}

	if capturedUpload.TriggerType != "manual" {
		t.Errorf("Expected trigger type 'manual', got %q", capturedUpload.TriggerType)
	}

	if capturedUpload.StartedAt.IsZero() {
		t.Error("Expected StartedAt to be set")
	}
}

func TestShouldSkipUpload_DatabaseHasRunning(t *testing.T) {
	executor := &mockExecutor{}

	db := &mockDatabase{
		getRunningUploadForNodeFunc: func(ctx context.Context, nodeName string) (*Upload, error) {
			return &Upload{
				ID:       789,
				NodeName: nodeName,
				Status:   "running",
			}, nil
		},
	}

	manager := NewManager(executor, db, logrus.New())
	shouldSkip, err := manager.ShouldSkipUpload(context.Background(), "test-node")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !shouldSkip {
		t.Error("Expected shouldSkip=true when database has running upload")
	}
}

func TestShouldSkipUpload_CommandShowsRunning(t *testing.T) {
	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			return `status:           Running
progress:         50.00% (1624/3248 uploading)`, "", nil
		},
	}

	db := &mockDatabase{
		getRunningUploadForNodeFunc: func(ctx context.Context, nodeName string) (*Upload, error) {
			return nil, nil // No running upload in database
		},
	}

	manager := NewManager(executor, db, logrus.New())
	shouldSkip, err := manager.ShouldSkipUpload(context.Background(), "test-node")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !shouldSkip {
		t.Error("Expected shouldSkip=true when command shows running upload")
	}
}

func TestShouldSkipUpload_NoRunningUpload(t *testing.T) {
	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			return `status:           2025-12-07 13:41:43 UTC| Finished with exit code 0
progress:         100.00% (3248/3248 completed)`, "", nil
		},
	}

	db := &mockDatabase{
		getRunningUploadForNodeFunc: func(ctx context.Context, nodeName string) (*Upload, error) {
			return nil, nil
		},
	}

	manager := NewManager(executor, db, logrus.New())
	shouldSkip, err := manager.ShouldSkipUpload(context.Background(), "test-node")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if shouldSkip {
		t.Error("Expected shouldSkip=false when no upload is running")
	}
}

func TestMonitorUploadProgress_StoresProgress(t *testing.T) {
	var capturedProgress UploadProgress

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			return `status:           Running
progress:         75.00% (2436/3248 uploading)`, "", nil
		},
	}

	db := &mockDatabase{
		storeUploadProgressFunc: func(ctx context.Context, progress UploadProgress) error {
			capturedProgress = progress
			return nil
		},
	}

	manager := NewManager(executor, db, logrus.New())
	err := manager.MonitorUploadProgress(context.Background(), 999, "test-node")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if capturedProgress.UploadID != 999 {
		t.Errorf("Expected upload ID 999, got %d", capturedProgress.UploadID)
	}

	if capturedProgress.CheckedAt.IsZero() {
		t.Error("Expected CheckedAt to be set")
	}

	if capturedProgress.ProgressData == nil {
		t.Error("Expected ProgressData to be set")
	}
}

func TestMonitorUploadProgress_UpdatesOnCompletion(t *testing.T) {
	var capturedUpload Upload

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			return `status:           2025-12-07 13:41:43 UTC| Finished with exit code 0
progress:         100.00% (3248/3248 completed)`, "", nil
		},
	}

	db := &mockDatabase{
		storeUploadProgressFunc: func(ctx context.Context, progress UploadProgress) error {
			return nil
		},
		updateUploadFunc: func(ctx context.Context, upload Upload) error {
			capturedUpload = upload
			return nil
		},
	}

	manager := NewManager(executor, db, logrus.New())
	err := manager.MonitorUploadProgress(context.Background(), 888, "test-node")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if capturedUpload.ID != 888 {
		t.Errorf("Expected upload ID 888, got %d", capturedUpload.ID)
	}

	if capturedUpload.Status != "completed" {
		t.Errorf("Expected status 'completed', got %q", capturedUpload.Status)
	}

	if capturedUpload.CompletedAt == nil {
		t.Error("Expected CompletedAt to be set")
	}
}

func TestInitiateUpload_ErrorHandling(t *testing.T) {
	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			return "", "command failed", errors.New("execution error")
		},
	}

	db := &mockDatabase{}

	manager := NewManager(executor, db, logrus.New())
	_, err := manager.InitiateUpload(context.Background(), "test-node", "scheduled")

	if err == nil {
		t.Error("Expected error when command execution fails")
	}
}

func TestCheckUploadStatus_ErrorHandling(t *testing.T) {
	manager := NewManager(&mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			return "", "error", errors.New("command failed")
		},
	}, &mockDatabase{}, logrus.New())

	status, err := manager.CheckUploadStatus(context.Background(), "test-node")

	// Generic command failures should return an error (not treat as "not running")
	if err == nil {
		t.Error("Expected error when command execution fails with generic error")
	}

	// Status should be nil when there's an error
	if status != nil {
		t.Error("Expected status to be nil when command execution fails")
	}
}

func TestMonitorUploadProgress_ContinuesOnRunning(t *testing.T) {
	var updateCalled bool

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			return `status:           Running
progress:         50.00% (1624/3248 uploading)`, "", nil
		},
	}

	db := &mockDatabase{
		storeUploadProgressFunc: func(ctx context.Context, progress UploadProgress) error {
			return nil
		},
		updateUploadFunc: func(ctx context.Context, upload Upload) error {
			updateCalled = true
			return nil
		},
	}

	manager := NewManager(executor, db, logrus.New())
	err := manager.MonitorUploadProgress(context.Background(), 777, "test-node")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if updateCalled {
		t.Error("Expected UpdateUpload not to be called when upload is still running")
	}
}

func TestParseUploadStatus_ProgressExtraction(t *testing.T) {
	manager := NewManager(&mockExecutor{}, &mockDatabase{}, logrus.New())

	tests := []struct {
		name              string
		output            string
		expectedRunning   bool
		expectedPercent   string
		expectedCompleted string
		expectedTotal     string
	}{
		{
			name: "Completed upload with full details",
			output: `status:           2025-12-07 13:41:43 UTC| Finished with exit code 0
progress:         100.00% (3248/3248 multi-client upload completed)
restart_count:    0`,
			expectedRunning:   false,
			expectedPercent:   "100.00",
			expectedCompleted: "3248",
			expectedTotal:     "3248",
		},
		{
			name: "Running upload with progress",
			output: `status:           Running
progress:         75.50% (3100/4112 uploading)
restart_count:    1`,
			expectedRunning:   true,
			expectedPercent:   "75.50",
			expectedCompleted: "3100",
			expectedTotal:     "4112",
		},
		{
			name: "Partial progress",
			output: `status:           Running
progress:         25.00% (1024/4096 in progress)`,
			expectedRunning:   true,
			expectedPercent:   "25.00",
			expectedCompleted: "1024",
			expectedTotal:     "4096",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := manager.parseUploadStatus(tt.output)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if status.IsRunning != tt.expectedRunning {
				t.Errorf("Expected IsRunning=%v, got %v", tt.expectedRunning, status.IsRunning)
			}
			if tt.expectedPercent != "" {
				if percent, ok := status.Progress["progress_percent"].(string); !ok || percent != tt.expectedPercent {
					t.Errorf("Expected progress_percent=%q, got %v", tt.expectedPercent, status.Progress["progress_percent"])
				}
			}
			if tt.expectedCompleted != "" {
				if completed, ok := status.Progress["chunks_completed"].(string); !ok || completed != tt.expectedCompleted {
					t.Errorf("Expected chunks_completed=%q, got %v", tt.expectedCompleted, status.Progress["chunks_completed"])
				}
			}
			if tt.expectedTotal != "" {
				if total, ok := status.Progress["chunks_total"].(string); !ok || total != tt.expectedTotal {
					t.Errorf("Expected chunks_total=%q, got %v", tt.expectedTotal, status.Progress["chunks_total"])
				}
			}
		})
	}
}
func TestCheckUploadStatus_JobNotFound(t *testing.T) {
	// Test the case where upload job has never been run before
	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
			// Simulate the error output when job has never been run
			stderr = `Error: status: Unknown, message: "status: Internal, message: \"job_status failed: unknown status, job 'upload' not found\", details: [], metadata: MetadataMap { headers: {\"content-type\": \"application/grpc\", \"date\": \"Wed, 10 Dec 2025 03:45:54 GMT\", \"content-length\": \"0\"} }", details: [], metadata: MetadataMap { headers: {"content-type": "application/grpc", "date": "Wed, 10 Dec 2025 03:45:54 GMT", "content-length": "0"} }`
			return "", stderr, errors.New("exit status 1")
		},
	}

	db := &mockDatabase{}
	manager := NewManager(executor, db, logrus.New())

	status, err := manager.CheckUploadStatus(context.Background(), "test-node")

	if err != nil {
		t.Fatalf("Expected no error when job not found, got: %v", err)
	}

	if status.IsRunning {
		t.Error("Expected IsRunning to be false when job not found")
	}

	if status.Progress["error"] == nil {
		t.Error("Expected error information to be stored in progress")
	}
}

func TestCheckUploadStatus_CommandError(t *testing.T) {
	// Test various command error scenarios
	testCases := []struct {
		name   string
		stderr string
		stdout string
		err    error
	}{
		{
			name:   "Job not found",
			stderr: "job 'upload' not found",
			err:    errors.New("exit status 1"),
		},
		{
			name:   "Unknown status",
			stderr: "unknown status",
			err:    errors.New("exit status 1"),
		},
		{
			name:   "Job status failed",
			stderr: "job_status failed: some error",
			err:    errors.New("exit status 1"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			executor := &mockExecutor{
				executeFunc: func(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
					return tc.stdout, tc.stderr, tc.err
				},
			}

			db := &mockDatabase{}
			manager := NewManager(executor, db, logrus.New())

			status, err := manager.CheckUploadStatus(context.Background(), "test-node")

			if err != nil {
				t.Fatalf("Expected no error for %s, got: %v", tc.name, err)
			}

			if status.IsRunning {
				t.Errorf("Expected IsRunning to be false for %s", tc.name)
			}
		})
	}
}
