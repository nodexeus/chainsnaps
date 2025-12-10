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
	InitiateUploadWithProtocolData(ctx context.Context, nodeName string, triggerType string, protocol string, nodeType string, protocolData map[string]interface{}) (int64, error)
	CreateUploadRecord(ctx context.Context, nodeName, protocol, nodeType, triggerType string, protocolData map[string]interface{}) (int64, error)
	CreateUploadRecordWithProgress(ctx context.Context, nodeName, protocol, nodeType, triggerType string, protocolData map[string]interface{}, progressData map[string]interface{}) (int64, error)
	MonitorUploadProgress(ctx context.Context, uploadID int64, nodeName string) error
	MonitorUploadProgressWithNotification(ctx context.Context, uploadID int64, nodeName string) (completed bool, err error)
	CheckUploadStatus(ctx context.Context, nodeName string) (*upload.UploadStatus, error)
}

// Database interface for database operations
type Database interface {
	CreateUpload(ctx context.Context, upload database.Upload) (int64, error)
	UpdateUpload(ctx context.Context, upload database.Upload) error
	GetRunningUploads(ctx context.Context) ([]database.Upload, error)
	GetRunningUploadForNode(ctx context.Context, nodeName string) (*database.Upload, error)
	GetLatestCompletedUploadForNode(ctx context.Context, nodeName string) (*database.Upload, error)
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

	// Step 3: Initiate upload with protocol data (metrics become part of upload record)
	uploadID, err := j.uploadManager.InitiateUploadWithProtocolData(ctx, j.nodeName, "scheduled", j.nodeConfig.Protocol, j.nodeConfig.Type, metrics)
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

	// Step 5: Upload initiated successfully
	// Monitoring will be handled by the UploadMonitorJob
	// Note: Completion notifications will be sent when the upload actually finishes

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
	uploadManager    UploadManager
	db               Database
	protocolRegistry *protocol.Registry
	notifyRegistry   *notification.Registry
	logger           *logrus.Logger
	nodeConfigs      map[string]config.NodeConfig
}

// NewUploadMonitorJob creates a new upload monitor job
func NewUploadMonitorJob(
	uploadManager UploadManager,
	db Database,
	protocolRegistry *protocol.Registry,
	notifyRegistry *notification.Registry,
	nodeConfigs map[string]config.NodeConfig,
	logger *logrus.Logger,
) *UploadMonitorJob {
	if logger == nil {
		logger = logrus.New()
	}

	return &UploadMonitorJob{
		uploadManager:    uploadManager,
		db:               db,
		protocolRegistry: protocolRegistry,
		notifyRegistry:   notifyRegistry,
		logger:           logger,
		nodeConfigs:      nodeConfigs,
	}
}

// Run executes the upload monitoring workflow
func (j *UploadMonitorJob) Run(ctx context.Context) error {
	j.logger.WithFields(logrus.Fields{
		"component": "scheduler",
		"job":       "upload_monitor",
	}).Debug("Starting comprehensive upload monitor job")

	// Step 1: Get all running uploads from database first
	runningUploads, err := j.db.GetRunningUploads(ctx)
	if err != nil {
		j.logger.WithFields(logrus.Fields{
			"component": "scheduler",
			"error":     err.Error(),
		}).Error("Failed to get running uploads")
		return fmt.Errorf("failed to get running uploads: %w", err)
	}

	// Step 2: Check for external uploads (running uploads not in database)
	trackedNodes := make(map[string]bool)
	for _, upload := range runningUploads {
		trackedNodes[upload.NodeName] = true
	}

	// Check all configured nodes for external uploads
	var discoveryWg sync.WaitGroup
	for nodeName := range j.nodeConfigs {
		// Skip nodes that already have tracked uploads
		if trackedNodes[nodeName] {
			continue
		}

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

			// Only create record for truly external uploads (not already tracked)
			if status.IsRunning {
				nodeConfig := j.nodeConfigs[node]

				// Collect protocol metrics for discovered uploads (blockchain state only)
				var protocolData map[string]interface{}
				if protocolModule, err := j.protocolRegistry.Get(nodeConfig.Protocol); err == nil {
					metrics, err := protocolModule.CollectMetrics(ctx, nodeConfig)
					if err != nil {
						j.logger.WithFields(logrus.Fields{
							"component": "scheduler",
							"node":      node,
							"protocol":  nodeConfig.Protocol,
							"error":     err.Error(),
						}).Warn("Failed to collect protocol metrics for discovered upload, using empty protocol data")

						// Use empty protocol data if collection fails
						protocolData = make(map[string]interface{})
					} else {
						// Use only the protocol metrics (blockchain state)
						protocolData = metrics
					}
				} else {
					// No protocol module, use empty protocol data
					j.logger.WithFields(logrus.Fields{
						"component": "scheduler",
						"node":      node,
						"protocol":  nodeConfig.Protocol,
					}).Warn("No protocol module found for discovered upload")
					protocolData = make(map[string]interface{})
				}

				// Extract progress data separately (for database columns)
				progressData := status.Progress

				uploadID, err := j.uploadManager.CreateUploadRecordWithProgress(ctx, node, nodeConfig.Protocol, nodeConfig.Type, "discovered", protocolData, progressData)
				if err != nil {
					j.logger.WithFields(logrus.Fields{
						"component": "scheduler",
						"node":      node,
						"error":     err.Error(),
					}).Error("Failed to create upload record for discovered upload")
					return
				}

				j.logger.WithFields(logrus.Fields{
					"component": "scheduler",
					"node":      node,
					"upload_id": uploadID,
				}).Info("Discovered and registered upload with protocol data")
			}
		}(nodeName)
	}

	discoveryWg.Wait()

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
			completed, err := j.uploadManager.MonitorUploadProgressWithNotification(ctx, u.ID, u.NodeName)
			if err != nil {
				j.logger.WithFields(logrus.Fields{
					"component": "scheduler",
					"node":      u.NodeName,
					"upload_id": u.ID,
					"error":     err.Error(),
				}).Error("Failed to monitor upload progress")
				// Don't return error - continue monitoring other uploads (node isolation)
			} else if completed {
				// Send completion notification
				j.sendNotification(ctx, u.NodeName, notification.EventComplete, "Upload completed successfully", map[string]interface{}{
					"upload_id": u.ID,
					"node":      u.NodeName,
				})
			}
		}(upload)
	}

	monitorWg.Wait()

	j.logger.WithFields(logrus.Fields{
		"component": "scheduler",
	}).Debug("Comprehensive upload monitor job completed")

	return nil
}

// sendNotification sends a notification for upload events
func (j *UploadMonitorJob) sendNotification(ctx context.Context, nodeName string, event notification.NotificationEvent, message string, details map[string]interface{}) {
	if j.notifyRegistry == nil {
		return
	}

	// Get node-specific notification config
	nodeConfig, exists := j.nodeConfigs[nodeName]
	if !exists {
		return
	}

	notifyConfig := nodeConfig.Notifications
	if notifyConfig == nil {
		return
	}

	// Check if this event type should trigger a notification
	shouldNotify := false
	switch event {
	case notification.EventFailure:
		shouldNotify = notifyConfig.Failure
	case notification.EventSkip:
		shouldNotify = notifyConfig.Skip
	case notification.EventComplete:
		shouldNotify = notifyConfig.Complete
	}

	if !shouldNotify {
		return
	}

	// Send notification to all configured types
	for notificationType, typeConfig := range notifyConfig.Types {
		notificationModule, err := j.notifyRegistry.Get(notificationType)
		if err != nil {
			j.logger.WithFields(logrus.Fields{
				"component": "scheduler",
				"type":      notificationType,
			}).Warn("Notification module not found")
			continue
		}

		payload := notification.NotificationPayload{
			Event:     event,
			NodeName:  nodeName,
			Timestamp: time.Now(),
			Message:   message,
			Details:   details,
		}

		if err := notificationModule.Send(ctx, typeConfig.URL, payload); err != nil {
			j.logger.WithFields(logrus.Fields{
				"component": "scheduler",
				"type":      notificationType,
				"node":      nodeName,
				"error":     err.Error(),
			}).Error("Failed to send notification")
		}
	}
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
