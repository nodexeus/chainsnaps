package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nodexeus/agent/internal/config"
	"github.com/nodexeus/agent/internal/database"
	"github.com/nodexeus/agent/internal/notification"
	"github.com/nodexeus/agent/internal/protocol"
	"github.com/nodexeus/agent/internal/upload"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

// Job represents a scheduled task
type Job interface {
	// Run executes the job logic
	Run(ctx context.Context) error
}

// Scheduler manages cron-based job execution
type Scheduler interface {
	// AddJob registers a job with a cron schedule
	AddJob(schedule string, job Job) error

	// Start begins executing scheduled jobs
	Start()

	// Stop gracefully shuts down the scheduler
	Stop(ctx context.Context) error
}

// CronScheduler implements the Scheduler interface using robfig/cron
type CronScheduler struct {
	cron   *cron.Cron
	logger *logrus.Logger
	wg     sync.WaitGroup
	mu     sync.Mutex
}

// NewCronScheduler creates a new cron-based scheduler
func NewCronScheduler(logger *logrus.Logger) *CronScheduler {
	if logger == nil {
		logger = logrus.New()
	}

	return &CronScheduler{
		cron:   cron.New(cron.WithSeconds()),
		logger: logger,
	}
}

// AddJob registers a job with a cron schedule
func (s *CronScheduler) AddJob(schedule string, job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Wrap the job to handle panics and logging
	wrappedJob := func() {
		s.wg.Add(1)
		defer s.wg.Done()

		ctx := context.Background()

		defer func() {
			if r := recover(); r != nil {
				s.logger.WithFields(logrus.Fields{
					"component": "scheduler",
					"panic":     r,
				}).Error("Job panicked")
			}
		}()

		if err := job.Run(ctx); err != nil {
			s.logger.WithFields(logrus.Fields{
				"component": "scheduler",
				"error":     err.Error(),
			}).Error("Job execution failed")
		}
	}

	_, err := s.cron.AddFunc(schedule, wrappedJob)
	if err != nil {
		return fmt.Errorf("failed to add job with schedule %s: %w", schedule, err)
	}

	s.logger.WithFields(logrus.Fields{
		"component": "scheduler",
		"schedule":  schedule,
	}).Info("Job added to scheduler")

	return nil
}

// Start begins executing scheduled jobs
func (s *CronScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cron.Start()
	s.logger.WithFields(logrus.Fields{
		"component": "scheduler",
	}).Info("Scheduler started")
}

// Stop gracefully shuts down the scheduler
func (s *CronScheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	cronCtx := s.cron.Stop()
	s.mu.Unlock()

	s.logger.WithFields(logrus.Fields{
		"component": "scheduler",
	}).Info("Scheduler stopping, waiting for jobs to complete")

	// Wait for cron to stop
	<-cronCtx.Done()

	// Wait for all jobs to complete with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.WithFields(logrus.Fields{
			"component": "scheduler",
		}).Info("All jobs completed, scheduler stopped")
		return nil
	case <-ctx.Done():
		s.logger.WithFields(logrus.Fields{
			"component": "scheduler",
		}).Warn("Scheduler stop timeout, some jobs may not have completed")
		return ctx.Err()
	}
}

// UploadManager interface for upload operations
type UploadManager interface {
	ShouldSkipUpload(ctx context.Context, nodeName string) (bool, error)
	InitiateUpload(ctx context.Context, nodeName string, triggerType string) (int64, error)
	MonitorUploadProgress(ctx context.Context, uploadID int64, nodeName string) error
	CheckUploadStatus(ctx context.Context, nodeName string) (*upload.UploadStatus, error)
}

// Database interface for database operations
type Database interface {
	StoreNodeMetrics(ctx context.Context, metrics database.NodeMetrics) error
	CreateUpload(ctx context.Context, upload database.Upload) (int64, error)
	GetRunningUploads(ctx context.Context) ([]database.Upload, error)
	GetRunningUploadForNode(ctx context.Context, nodeName string) (*database.Upload, error)
}

// NodeUploadJob handles the upload workflow for a single node
type NodeUploadJob struct {
	nodeName         string
	nodeConfig       config.NodeConfig
	protocolRegistry *protocol.Registry
	uploadManager    UploadManager
	db               Database
	notifyRegistry   *notification.Registry
	notifyConfig     *config.NotificationConfig
	logger           *logrus.Logger
}

// NewNodeUploadJob creates a new node upload job
func NewNodeUploadJob(
	nodeName string,
	nodeConfig config.NodeConfig,
	protocolRegistry *protocol.Registry,
	uploadManager UploadManager,
	db Database,
	notifyRegistry *notification.Registry,
	notifyConfig *config.NotificationConfig,
	logger *logrus.Logger,
) *NodeUploadJob {
	if logger == nil {
		logger = logrus.New()
	}

	return &NodeUploadJob{
		nodeName:         nodeName,
		nodeConfig:       nodeConfig,
		protocolRegistry: protocolRegistry,
		uploadManager:    uploadManager,
		db:               db,
		notifyRegistry:   notifyRegistry,
		notifyConfig:     notifyConfig,
		logger:           logger,
	}
}

// Run executes the node upload workflow
func (j *NodeUploadJob) Run(ctx context.Context) error {
	j.logger.WithFields(logrus.Fields{
		"component": "scheduler",
		"job":       "node_upload",
		"node":      j.nodeName,
	}).Info("Starting node upload job")

	// Step 1: Check if upload is already running
	shouldSkip, err := j.uploadManager.ShouldSkipUpload(ctx, j.nodeName)
	if err != nil {
		j.logger.WithFields(logrus.Fields{
			"component": "scheduler",
			"node":      j.nodeName,
			"error":     err.Error(),
		}).Error("Failed to check upload status")
		j.sendNotification(ctx, notification.EventFailure, "Failed to check upload status", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to check upload status: %w", err)
	}

	if shouldSkip {
		j.logger.WithFields(logrus.Fields{
			"component": "scheduler",
			"node":      j.nodeName,
		}).Info("Upload already running, skipping")
		j.sendNotification(ctx, notification.EventSkip, "Upload already running", nil)
		return nil
	}

	// Step 2: Collect metrics via protocol module
	protocolModule, err := j.protocolRegistry.Get(j.nodeConfig.Protocol)
	if err != nil {
		j.logger.WithFields(logrus.Fields{
			"component": "scheduler",
			"node":      j.nodeName,
			"protocol":  j.nodeConfig.Protocol,
			"error":     err.Error(),
		}).Error("Failed to get protocol module")
		j.sendNotification(ctx, notification.EventFailure, "Failed to get protocol module", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to get protocol module: %w", err)
	}

	metrics, err := protocolModule.CollectMetrics(ctx, j.nodeConfig)
	if err != nil {
		j.logger.WithFields(logrus.Fields{
			"component": "scheduler",
			"node":      j.nodeName,
			"error":     err.Error(),
		}).Error("Failed to collect metrics")
		// Store partial metrics with null values for failed queries
		metrics = map[string]interface{}{
			"error": err.Error(),
		}
	}

	// Step 3: Store metrics in database
	nodeMetrics := database.NodeMetrics{
		NodeName:    j.nodeName,
		Protocol:    j.nodeConfig.Protocol,
		NodeType:    j.nodeConfig.Type,
		CollectedAt: time.Now(),
		Metrics:     database.JSONB(metrics),
	}

	if err := j.db.StoreNodeMetrics(ctx, nodeMetrics); err != nil {
		j.logger.WithFields(logrus.Fields{
			"component": "scheduler",
			"node":      j.nodeName,
			"error":     err.Error(),
		}).Error("Failed to store metrics")
		j.sendNotification(ctx, notification.EventFailure, "Failed to store metrics", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to store metrics: %w", err)
	}

	j.logger.WithFields(logrus.Fields{
		"component": "scheduler",
		"node":      j.nodeName,
		"metrics":   metrics,
	}).Info("Metrics collected and stored")

	// Step 4: Initiate upload
	uploadID, err := j.uploadManager.InitiateUpload(ctx, j.nodeName, "scheduled")
	if err != nil {
		j.logger.WithFields(logrus.Fields{
			"component": "scheduler",
			"node":      j.nodeName,
			"error":     err.Error(),
		}).Error("Failed to initiate upload")
		j.sendNotification(ctx, notification.EventFailure, "Failed to initiate upload", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to initiate upload: %w", err)
	}

	j.logger.WithFields(logrus.Fields{
		"component": "scheduler",
		"node":      j.nodeName,
		"upload_id": uploadID,
	}).Info("Upload initiated")

	// Step 5: Start monitoring upload progress
	// This will be handled by the UploadMonitorJob
	j.sendNotification(ctx, notification.EventComplete, "Upload workflow completed", map[string]interface{}{
		"upload_id": uploadID,
		"metrics":   metrics,
	})

	return nil
}

// sendNotification sends a notification if configured
func (j *NodeUploadJob) sendNotification(ctx context.Context, event notification.NotificationEvent, message string, details map[string]interface{}) {
	if j.notifyConfig == nil || j.notifyRegistry == nil {
		return
	}

	// Check if this event type should trigger a notification
	shouldNotify := false
	switch event {
	case notification.EventFailure:
		shouldNotify = j.notifyConfig.Failure
	case notification.EventSkip:
		shouldNotify = j.notifyConfig.Skip
	case notification.EventComplete:
		shouldNotify = j.notifyConfig.Complete
	}

	if !shouldNotify {
		return
	}

	// Send the notification to all configured types
	payload := notification.NotificationPayload{
		Event:     event,
		NodeName:  j.nodeName,
		Timestamp: time.Now(),
		Message:   message,
		Details:   details,
	}

	// Iterate through all configured notification types
	for notificationType := range j.notifyConfig.Types {
		notifyModule, err := j.notifyRegistry.Get(notificationType)
		if err != nil {
			j.logger.WithFields(logrus.Fields{
				"component":         "scheduler",
				"node":              j.nodeName,
				"notification_type": notificationType,
				"error":             err.Error(),
			}).Error("Failed to get notification module")
			continue
		}

		url := j.notifyConfig.GetNotificationURL(notificationType)
		if url == "" {
			j.logger.WithFields(logrus.Fields{
				"component":         "scheduler",
				"node":              j.nodeName,
				"notification_type": notificationType,
			}).Warn("No URL configured for notification type")
			continue
		}

		if err := notifyModule.Send(ctx, url, payload); err != nil {
			j.logger.WithFields(logrus.Fields{
				"component":         "scheduler",
				"node":              j.nodeName,
				"notification_type": notificationType,
				"error":             err.Error(),
			}).Error("Failed to send notification")
		}
	}
}

// UploadMonitorJob monitors all running uploads and updates their progress
type UploadMonitorJob struct {
	uploadManager UploadManager
	db            Database
	logger        *logrus.Logger
	nodeConfigs   map[string]config.NodeConfig
}

// NewUploadMonitorJob creates a new upload monitor job
func NewUploadMonitorJob(
	uploadManager UploadManager,
	db Database,
	nodeConfigs map[string]config.NodeConfig,
	logger *logrus.Logger,
) *UploadMonitorJob {
	if logger == nil {
		logger = logrus.New()
	}

	return &UploadMonitorJob{
		uploadManager: uploadManager,
		db:            db,
		logger:        logger,
		nodeConfigs:   nodeConfigs,
	}
}

// Run executes the upload monitoring workflow
func (j *UploadMonitorJob) Run(ctx context.Context) error {
	j.logger.WithFields(logrus.Fields{
		"component": "scheduler",
		"job":       "upload_monitor",
	}).Debug("Starting comprehensive upload monitor job")

	// Step 1: Check all configured nodes for running uploads (even if not in database)
	discoveredUploads := make(map[string]bool) // nodeName -> isRunning
	var discoveryWg sync.WaitGroup

	for nodeName := range j.nodeConfigs {
		discoveryWg.Add(1)
		go func(node string) {
			defer discoveryWg.Done()

			// Check if this node has a running upload
			status, err := j.uploadManager.CheckUploadStatus(ctx, node)
			if err != nil {
				j.logger.WithFields(logrus.Fields{
					"component": "scheduler",
					"node":      node,
					"error":     err.Error(),
				}).Warn("Failed to check upload status for node")
				return
			}

			discoveredUploads[node] = status.IsRunning

			// If upload is running but not in database, create a record
			if status.IsRunning {
				existingUpload, err := j.db.GetRunningUploadForNode(ctx, node)
				if err != nil {
					j.logger.WithFields(logrus.Fields{
						"component": "scheduler",
						"node":      node,
						"error":     err.Error(),
					}).Warn("Failed to check database for existing upload")
					return
				}

				if existingUpload == nil {
					// Create database record for externally started upload
					// Convert upload.JSONB to database.JSONB
					dbProgress := make(database.JSONB)
					for k, v := range status.Progress {
						dbProgress[k] = v
					}

					// Extract structured progress data
					var progressPercent *float64
					var chunksCompleted *int
					var chunksTotal *int

					if percentStr, ok := status.Progress["progress_percent"].(string); ok {
						if percent, err := parseFloat(percentStr); err == nil {
							progressPercent = &percent
						}
					}
					if completedStr, ok := status.Progress["chunks_completed"].(string); ok {
						if completed, err := parseInt(completedStr); err == nil {
							chunksCompleted = &completed
						}
					}
					if totalStr, ok := status.Progress["chunks_total"].(string); ok {
						if total, err := parseInt(totalStr); err == nil {
							chunksTotal = &total
						}
					}

					upload := database.Upload{
						NodeName:        node,
						StartedAt:       time.Now(),
						Status:          "running",
						Progress:        dbProgress,
						ProgressPercent: progressPercent,
						ChunksCompleted: chunksCompleted,
						ChunksTotal:     chunksTotal,
						TriggerType:     "external",
					}

					uploadID, err := j.db.CreateUpload(ctx, upload)
					if err != nil {
						j.logger.WithFields(logrus.Fields{
							"component": "scheduler",
							"node":      node,
							"error":     err.Error(),
						}).Error("Failed to create database record for external upload")
						return
					}

					j.logger.WithFields(logrus.Fields{
						"component": "scheduler",
						"node":      node,
						"upload_id": uploadID,
					}).Info("Discovered and registered external upload")
				}
			}
		}(nodeName)
	}

	discoveryWg.Wait()

	// Step 2: Get all running uploads from database (including newly discovered ones)
	runningUploads, err := j.db.GetRunningUploads(ctx)
	if err != nil {
		j.logger.WithFields(logrus.Fields{
			"component": "scheduler",
			"error":     err.Error(),
		}).Error("Failed to get running uploads")
		return fmt.Errorf("failed to get running uploads: %w", err)
	}

	if len(runningUploads) == 0 {
		j.logger.WithFields(logrus.Fields{
			"component": "scheduler",
		}).Debug("No running uploads to monitor")
		return nil
	}

	j.logger.WithFields(logrus.Fields{
		"component": "scheduler",
		"count":     len(runningUploads),
	}).Info("Monitoring running uploads")

	// Step 3: Monitor each upload independently (node isolation)
	var monitorWg sync.WaitGroup
	for _, upload := range runningUploads {
		monitorWg.Add(1)
		go func(u database.Upload) {
			defer monitorWg.Done()

			// Each upload is monitored independently to ensure node isolation
			if err := j.uploadManager.MonitorUploadProgress(ctx, u.ID, u.NodeName); err != nil {
				j.logger.WithFields(logrus.Fields{
					"component": "scheduler",
					"node":      u.NodeName,
					"upload_id": u.ID,
					"error":     err.Error(),
				}).Error("Failed to monitor upload progress")
				// Don't return error - continue monitoring other uploads (node isolation)
			}
		}(upload)
	}

	monitorWg.Wait()

	j.logger.WithFields(logrus.Fields{
		"component": "scheduler",
	}).Debug("Comprehensive upload monitor job completed")

	return nil
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
