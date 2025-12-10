package upload

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// CommandExecutor interface for executing system commands
type CommandExecutor interface {
	Execute(ctx context.Context, command string, args ...string) (stdout, stderr string, err error)
}

// JSONB represents a JSONB column type
type JSONB map[string]interface{}

// Upload represents an upload operation
type Upload struct {
	ID                int64
	NodeName          string
	Protocol          string
	NodeType          string
	StartedAt         time.Time
	CompletedAt       *time.Time
	Status            string
	TriggerType       string
	ErrorMessage      *string
	ProtocolData      JSONB   // Blockchain state when upload started
	TotalChunks       *int    // Total chunks in completed upload
	CompletionMessage *string // Success/completion message
}

// UploadProgress represents a progress check during an upload
type UploadProgress struct {
	ID              int64
	UploadID        int64
	CheckedAt       time.Time
	ProgressPercent *float64
	ChunksCompleted *int
	ChunksTotal     *int
	RawStatus       *string
}

// Database interface for upload persistence
type Database interface {
	CreateUpload(ctx context.Context, upload Upload) (int64, error)
	UpdateUpload(ctx context.Context, upload Upload) error
	GetRunningUploadForNode(ctx context.Context, nodeName string) (*Upload, error)
	GetLatestCompletedUploadForNode(ctx context.Context, nodeName string) (*Upload, error)
	StoreUploadProgress(ctx context.Context, progress UploadProgress) error
}

// UploadStatus represents the parsed status from the info command
type UploadStatus struct {
	IsRunning bool
	Progress  JSONB
}

// Manager handles upload operations
type Manager struct {
	executor CommandExecutor
	db       Database
	logger   *logrus.Logger
}

// NewManager creates a new upload manager
func NewManager(executor CommandExecutor, db Database, logger *logrus.Logger) *Manager {
	if logger == nil {
		logger = logrus.New()
	}
	return &Manager{
		executor: executor,
		db:       db,
		logger:   logger,
	}
}

// CheckUploadStatus checks if an upload is currently running for a node
func (m *Manager) CheckUploadStatus(ctx context.Context, nodeName string) (*UploadStatus, error) {
	m.logger.WithFields(logrus.Fields{
		"component": "upload",
		"node":      nodeName,
		"action":    "check_status",
	}).Debug("Checking upload status")

	// Execute: bv node job <node> info upload
	stdout, stderr, err := m.executor.Execute(ctx, "bv", "node", "job", nodeName, "info", "upload")
	if err != nil {
		// Check if this is a "job not found" type error vs other system errors
		errorOutput := stderr
		if errorOutput == "" {
			errorOutput = stdout
		}

		lowerError := strings.ToLower(errorOutput)
		lowerErrMsg := strings.ToLower(err.Error())

		// Only treat specific "job not found" errors as "not running"
		if strings.Contains(lowerError, "job 'upload' not found") ||
			strings.Contains(lowerError, "unknown status") ||
			strings.Contains(lowerError, "job_status failed") ||
			strings.Contains(lowerErrMsg, "job 'upload' not found") ||
			strings.Contains(lowerErrMsg, "unknown status") {

			m.logger.WithFields(logrus.Fields{
				"component": "upload",
				"node":      nodeName,
				"error":     err.Error(),
				"stderr":    stderr,
			}).Debug("Upload job not found, treating as not running")

			status := &UploadStatus{
				IsRunning: false,
				Progress: JSONB{
					"error":      err.Error(),
					"stderr":     stderr,
					"stdout":     stdout,
					"raw_output": errorOutput,
				},
			}
			return status, nil
		}

		// For other errors, return the error
		// Don't assume the upload status based on command execution issues
		m.logger.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"error":     err.Error(),
			"stderr":    stderr,
		}).Error("Failed to check upload status")
		return nil, fmt.Errorf("failed to check upload status: %w", err)
	}

	// Parse the status from stdout
	status, err := m.parseUploadStatus(stdout)
	if err != nil {
		m.logger.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"error":     err.Error(),
			"stdout":    stdout,
		}).Error("Failed to parse upload status")
		return nil, fmt.Errorf("failed to parse upload status: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"component":  "upload",
		"node":       nodeName,
		"is_running": status.IsRunning,
	}).Info("Upload status checked")

	return status, nil
}

// parseUploadStatus parses the output from the upload info command
// Expected format from `bv node job <node> info upload`:
// status:           2025-12-07 13:41:43 UTC| Finished with exit code 0 and message `...`
// progress:         100.00% (3248/3248 multi-client upload completed)
// restart_count:    0
// upgrade_blocking: true
// logs:             <empty>
func (m *Manager) parseUploadStatus(output string) (*UploadStatus, error) {
	output = strings.TrimSpace(output)

	status := &UploadStatus{
		Progress: make(JSONB),
	}

	// Check for empty output or no job indicators
	lowerOutput := strings.ToLower(output)
	if output == "" ||
		strings.Contains(lowerOutput, "no job") ||
		strings.Contains(lowerOutput, "no upload") ||
		strings.Contains(lowerOutput, "not found") ||
		strings.Contains(lowerOutput, "job 'upload' not found") ||
		strings.Contains(lowerOutput, "unknown status") ||
		strings.Contains(lowerOutput, "job_status failed") {
		status.IsRunning = false
		status.Progress["raw_output"] = output
		return status, nil
	}

	// Parse the key-value format
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split on first colon
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "status":
			status.Progress["status"] = value
			// Check if status indicates running
			lowerValue := strings.ToLower(value)
			if strings.Contains(lowerValue, "running") {
				status.IsRunning = true
			} else if strings.Contains(lowerValue, "finished") ||
				strings.Contains(lowerValue, "completed") ||
				strings.Contains(lowerValue, "failed") ||
				strings.Contains(lowerValue, "exit code") ||
				strings.Contains(lowerValue, "unknown") ||
				strings.Contains(lowerValue, "error") {
				status.IsRunning = false
			}

		case "progress":
			status.Progress["progress"] = value
			// Extract percentage if present
			if strings.Contains(value, "%") {
				// Parse format like "100.00% (3248/3248 ...)"
				percentIdx := strings.Index(value, "%")
				if percentIdx > 0 {
					percentStr := strings.TrimSpace(value[:percentIdx])
					status.Progress["progress_percent"] = percentStr
				}

				// Extract chunk counts if present (e.g., "3100/3248")
				if strings.Contains(value, "(") && strings.Contains(value, "/") {
					startIdx := strings.Index(value, "(")
					endIdx := strings.Index(value, ")")
					if startIdx > 0 && endIdx > startIdx {
						chunkInfo := value[startIdx+1 : endIdx]
						slashIdx := strings.Index(chunkInfo, "/")
						if slashIdx > 0 {
							completed := strings.TrimSpace(chunkInfo[:slashIdx])
							remaining := chunkInfo[slashIdx+1:]
							spaceIdx := strings.Index(remaining, " ")
							if spaceIdx > 0 {
								remaining = remaining[:spaceIdx]
							}
							status.Progress["chunks_completed"] = completed
							status.Progress["chunks_total"] = strings.TrimSpace(remaining)
						}
					}
				}
			}

		case "restart_count":
			status.Progress["restart_count"] = value

		case "upgrade_blocking":
			status.Progress["upgrade_blocking"] = value

		case "logs":
			status.Progress["logs"] = value
		}
	}

	// Store raw output for debugging
	status.Progress["raw_output"] = output

	return status, nil
}

// extractProgressData extracts structured progress data from parsed status
func (m *Manager) extractProgressData(progress JSONB) (progressPercent *float64, chunksCompleted *int, chunksTotal *int) {
	// Extract progress percentage
	if percentStr, ok := progress["progress_percent"].(string); ok {
		if percent, err := parseFloat(percentStr); err == nil {
			progressPercent = &percent
		}
	}

	// Extract chunks completed
	if completedStr, ok := progress["chunks_completed"].(string); ok {
		if completed, err := parseInt(completedStr); err == nil {
			chunksCompleted = &completed
		}
	}

	// Extract chunks total
	if totalStr, ok := progress["chunks_total"].(string); ok {
		if total, err := parseInt(totalStr); err == nil {
			chunksTotal = &total
		}
	}

	return progressPercent, chunksCompleted, chunksTotal
}

// parseFloat safely parses a string to float64
func parseFloat(s string) (float64, error) {
	// Remove any trailing characters like '%'
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")

	// Try to parse as float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}
	return 0, fmt.Errorf("invalid float: %s", s)
}

// parseInt safely parses a string to int
func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if i, err := strconv.Atoi(s); err == nil {
		return i, nil
	}
	return 0, fmt.Errorf("invalid int: %s", s)
}

// InitiateUploadWithProtocolData starts a new upload for a node with protocol data
func (m *Manager) InitiateUploadWithProtocolData(ctx context.Context, nodeName string, triggerType string, protocol string, nodeType string, protocolData map[string]interface{}) (int64, error) {
	m.logger.WithFields(logrus.Fields{
		"component":    "upload",
		"node":         nodeName,
		"protocol":     protocol,
		"trigger_type": triggerType,
		"action":       "initiate_with_protocol_data",
	}).Info("Initiating upload with protocol data")

	// Execute: bv node run upload <node>
	stdout, stderr, err := m.executor.Execute(ctx, "bv", "node", "run", "upload", nodeName)
	if err != nil {
		m.logger.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"error":     err.Error(),
			"stderr":    stderr,
			"stdout":    stdout,
		}).Error("Failed to initiate upload")
		return 0, fmt.Errorf("failed to initiate upload: %w", err)
	}

	// Create upload record in database with protocol data
	upload := Upload{
		NodeName:     nodeName,
		Protocol:     protocol,
		NodeType:     nodeType,
		StartedAt:    time.Now(),
		Status:       "running",
		TriggerType:  triggerType,
		ProtocolData: JSONB(protocolData),
	}

	uploadID, err := m.db.CreateUpload(ctx, upload)
	if err != nil {
		m.logger.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"error":     err.Error(),
		}).Error("Failed to create upload record")
		return 0, fmt.Errorf("failed to create upload record: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"component":     "upload",
		"node":          nodeName,
		"upload_id":     uploadID,
		"protocol_data": protocolData,
	}).Info("Upload initiated successfully with protocol data")

	return uploadID, nil
}

// InitiateUpload starts a new upload for a node (legacy method)
func (m *Manager) InitiateUpload(ctx context.Context, nodeName string, triggerType string) (int64, error) {
	m.logger.WithFields(logrus.Fields{
		"component":    "upload",
		"node":         nodeName,
		"trigger_type": triggerType,
		"action":       "initiate",
	}).Info("Initiating upload")

	// Execute: bv node run upload <node>
	stdout, stderr, err := m.executor.Execute(ctx, "bv", "node", "run", "upload", nodeName)
	if err != nil {
		m.logger.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"error":     err.Error(),
			"stderr":    stderr,
		}).Error("Failed to initiate upload")
		return 0, fmt.Errorf("failed to initiate upload: %w", err)
	}

	// Create upload record in database (legacy method - minimal protocol data)
	upload := Upload{
		NodeName:     nodeName,
		Protocol:     "unknown", // Legacy uploads don't have protocol info
		NodeType:     "unknown",
		StartedAt:    time.Now(),
		Status:       "running",
		TriggerType:  triggerType,
		ProtocolData: JSONB{"stdout": stdout, "stderr": stderr, "legacy": true},
	}

	uploadID, err := m.db.CreateUpload(ctx, upload)
	if err != nil {
		m.logger.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"error":     err.Error(),
		}).Error("Failed to create upload record")
		return 0, fmt.Errorf("failed to create upload record: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"component": "upload",
		"node":      nodeName,
		"upload_id": uploadID,
	}).Info("Upload initiated successfully")

	return uploadID, nil
}

// MonitorUploadProgress checks and updates the progress of an upload
func (m *Manager) MonitorUploadProgress(ctx context.Context, uploadID int64, nodeName string) error {
	m.logger.WithFields(logrus.Fields{
		"component": "upload",
		"node":      nodeName,
		"upload_id": uploadID,
		"action":    "monitor_progress",
	}).Debug("Monitoring upload progress")

	// Check current status
	status, err := m.CheckUploadStatus(ctx, nodeName)
	if err != nil {
		return fmt.Errorf("failed to check upload status: %w", err)
	}

	// Extract structured progress data
	progressPercent, chunksCompleted, chunksTotal := m.extractProgressData(status.Progress)

	// Store progress in upload_progress table
	var rawStatus *string
	if rawOutput, ok := status.Progress["raw_output"].(string); ok {
		rawStatus = &rawOutput
	}

	progress := UploadProgress{
		UploadID:        uploadID,
		CheckedAt:       time.Now(),
		ProgressPercent: progressPercent,
		ChunksCompleted: chunksCompleted,
		ChunksTotal:     chunksTotal,
		RawStatus:       rawStatus,
	}

	if err := m.db.StoreUploadProgress(ctx, progress); err != nil {
		m.logger.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"upload_id": uploadID,
			"error":     err.Error(),
		}).Warn("Failed to store upload progress")
		// Don't fail the whole operation if progress storage fails
	}

	// If upload is no longer running, update the main upload record with completion data
	if !status.IsRunning {
		now := time.Now()

		// Extract completion message
		var completionMessage *string
		if statusMsg, ok := status.Progress["status"].(string); ok {
			completionMessage = &statusMsg
		}

		upload := Upload{
			ID:                uploadID,
			Status:            "completed",
			CompletedAt:       &now,
			TotalChunks:       chunksTotal,
			CompletionMessage: completionMessage,
		}

		if err := m.db.UpdateUpload(ctx, upload); err != nil {
			m.logger.WithFields(logrus.Fields{
				"component": "upload",
				"node":      nodeName,
				"upload_id": uploadID,
				"error":     err.Error(),
			}).Error("Failed to update upload completion")
			return fmt.Errorf("failed to update upload completion: %w", err)
		}

		m.logger.WithFields(logrus.Fields{
			"component":          "upload",
			"node":               nodeName,
			"upload_id":          uploadID,
			"total_chunks":       chunksTotal,
			"completion_message": completionMessage,
		}).Info("Upload completed")
	}

	return nil
}

// ShouldSkipUpload checks if an upload should be skipped (already running)
func (m *Manager) ShouldSkipUpload(ctx context.Context, nodeName string) (bool, error) {
	// Check database for running upload
	runningUpload, err := m.db.GetRunningUploadForNode(ctx, nodeName)
	if err != nil {
		return false, fmt.Errorf("failed to check for running upload: %w", err)
	}

	if runningUpload != nil {
		m.logger.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"upload_id": runningUpload.ID,
		}).Info("Upload already running, skipping")
		return true, nil
	}

	// Also check via command to be sure
	status, err := m.CheckUploadStatus(ctx, nodeName)
	if err != nil {
		return false, fmt.Errorf("failed to check upload status: %w", err)
	}

	if status.IsRunning {
		m.logger.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
		}).Info("Upload detected as running via command, skipping")
		return true, nil
	}

	return false, nil
}
