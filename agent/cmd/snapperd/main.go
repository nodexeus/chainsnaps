package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nodexeus/agent/internal/config"
	"github.com/nodexeus/agent/internal/database"
	"github.com/nodexeus/agent/internal/executor"
	"github.com/nodexeus/agent/internal/logger"
	"github.com/nodexeus/agent/internal/notification"
	"github.com/nodexeus/agent/internal/protocol"
	"github.com/nodexeus/agent/internal/scheduler"
	"github.com/nodexeus/agent/internal/upload"
	"github.com/sirupsen/logrus"
)

var (
	// Version information (can be overridden at build time with -ldflags)
	version    = Version
	buildDate  = BuildDate
	commitHash = CommitHash
)

// DatabaseAdapter adapts database.DB to upload.Database interface
type DatabaseAdapter struct {
	db *database.DB
}

// CreateUpload adapts upload.Upload to database.Upload
func (a *DatabaseAdapter) CreateUpload(ctx context.Context, u upload.Upload) (int64, error) {
	dbUpload := database.Upload{
		NodeName:          u.NodeName,
		Protocol:          u.Protocol,
		NodeType:          u.NodeType,
		StartedAt:         u.StartedAt,
		Status:            u.Status,
		TriggerType:       u.TriggerType,
		ErrorMessage:      u.ErrorMessage,
		ProtocolData:      database.JSONB(u.ProtocolData),
		TotalChunks:       u.TotalChunks,
		CompletionMessage: u.CompletionMessage,
	}
	return a.db.CreateUpload(ctx, dbUpload)
}

// UpdateUpload adapts upload.Upload to database.Upload
func (a *DatabaseAdapter) UpdateUpload(ctx context.Context, u upload.Upload) error {
	dbUpload := database.Upload{
		ID:                u.ID,
		NodeName:          u.NodeName,
		Protocol:          u.Protocol,
		NodeType:          u.NodeType,
		StartedAt:         u.StartedAt,
		CompletedAt:       u.CompletedAt,
		Status:            u.Status,
		TriggerType:       u.TriggerType,
		ErrorMessage:      u.ErrorMessage,
		ProtocolData:      database.JSONB(u.ProtocolData),
		TotalChunks:       u.TotalChunks,
		CompletionMessage: u.CompletionMessage,
	}
	return a.db.UpdateUpload(ctx, dbUpload)
}

// GetRunningUploadForNode adapts database.Upload to upload.Upload
func (a *DatabaseAdapter) GetRunningUploadForNode(ctx context.Context, nodeName string) (*upload.Upload, error) {
	dbUpload, err := a.db.GetRunningUploadForNode(ctx, nodeName)
	if err != nil {
		return nil, err
	}
	if dbUpload == nil {
		return nil, nil
	}
	return &upload.Upload{
		ID:                dbUpload.ID,
		NodeName:          dbUpload.NodeName,
		Protocol:          dbUpload.Protocol,
		NodeType:          dbUpload.NodeType,
		StartedAt:         dbUpload.StartedAt,
		CompletedAt:       dbUpload.CompletedAt,
		Status:            dbUpload.Status,
		TriggerType:       dbUpload.TriggerType,
		ErrorMessage:      dbUpload.ErrorMessage,
		ProtocolData:      upload.JSONB(dbUpload.ProtocolData),
		TotalChunks:       dbUpload.TotalChunks,
		CompletionMessage: dbUpload.CompletionMessage,
	}, nil
}

// StoreUploadProgress adapts upload.UploadProgress to database.UploadProgress
func (a *DatabaseAdapter) StoreUploadProgress(ctx context.Context, p upload.UploadProgress) error {
	dbProgress := database.UploadProgress{
		UploadID:        p.UploadID,
		CheckedAt:       p.CheckedAt,
		ProgressPercent: p.ProgressPercent,
		ChunksCompleted: p.ChunksCompleted,
		ChunksTotal:     p.ChunksTotal,
		RawStatus:       p.RawStatus,
	}
	return a.db.StoreUploadProgress(ctx, dbProgress)
}

// GetLatestCompletedUploadForNode adapts database.Upload to upload.Upload
func (a *DatabaseAdapter) GetLatestCompletedUploadForNode(ctx context.Context, nodeName string) (*upload.Upload, error) {
	dbUpload, err := a.db.GetLatestCompletedUploadForNode(ctx, nodeName)
	if err != nil {
		return nil, err
	}
	if dbUpload == nil {
		return nil, nil
	}
	return &upload.Upload{
		ID:                dbUpload.ID,
		NodeName:          dbUpload.NodeName,
		Protocol:          dbUpload.Protocol,
		NodeType:          dbUpload.NodeType,
		StartedAt:         dbUpload.StartedAt,
		CompletedAt:       dbUpload.CompletedAt,
		Status:            dbUpload.Status,
		TriggerType:       dbUpload.TriggerType,
		ErrorMessage:      dbUpload.ErrorMessage,
		ProtocolData:      upload.JSONB(dbUpload.ProtocolData),
		TotalChunks:       dbUpload.TotalChunks,
		CompletionMessage: dbUpload.CompletionMessage,
	}, nil
}

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "/etc/snapperd/config.yaml", "Path to configuration file")
	consoleMode := flag.Bool("console", false, "Run in console mode with human-readable logs")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	// Handle version command
	if *showVersion {
		fmt.Printf("snapperd version %s\n", version)
		fmt.Printf("Build date: %s\n", buildDate)
		fmt.Printf("Commit: %s\n", commitHash)
		os.Exit(0)
	}

	// Handle subcommands
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "status":
			os.Exit(handleStatusCommand(*configPath, *consoleMode))
		case "upload":
			if len(args) < 2 {
				fmt.Fprintf(os.Stderr, "Error: upload command requires a node name\n")
				fmt.Fprintf(os.Stderr, "Usage: snapd upload <node>\n")
				os.Exit(1)
			}
			os.Exit(handleUploadCommand(*configPath, *consoleMode, args[1]))
		case "version":
			fmt.Printf("snapperd version %s\n", version)
			fmt.Printf("Build date: %s\n", buildDate)
			fmt.Printf("Commit: %s\n", commitHash)
			os.Exit(0)
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", args[0])
			fmt.Fprintf(os.Stderr, "Available commands: status, upload, version\n")
			os.Exit(1)
		}
	}

	// Run daemon mode
	os.Exit(runDaemon(*configPath, *consoleMode))
}

// runDaemon runs the daemon in either console or background mode
func runDaemon(configPath string, consoleMode bool) int {
	// Initialize logger
	log := logger.New(logger.Config{
		Level:       "info",
		ConsoleMode: consoleMode,
	})

	log.WithFields(logrus.Fields{
		"component":    "main",
		"version":      version,
		"build_date":   buildDate,
		"commit":       commitHash,
		"config_path":  configPath,
		"console_mode": consoleMode,
	}).Info("Starting snapshot daemon")

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "main",
			"error":     err.Error(),
		}).Error("Failed to load configuration")
		return 1
	}

	log.WithFields(logrus.Fields{
		"component":  "main",
		"node_count": len(cfg.Nodes),
	}).Info("Configuration loaded successfully")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database
	dbCfg := database.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		Database: cfg.Database.Database,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		SSLMode:  cfg.Database.SSLMode,
	}

	db, err := database.New(ctx, dbCfg)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "main",
			"error":     err.Error(),
		}).Error("Failed to connect to database")
		return 1
	}
	defer db.Close()

	log.WithFields(logrus.Fields{
		"component": "main",
	}).Info("Database connection established")

	// Run database migrations
	if err := db.Migrate(ctx); err != nil {
		log.WithFields(logrus.Fields{
			"component": "main",
			"error":     err.Error(),
		}).Error("Failed to run database migrations")
		return 1
	}

	log.WithFields(logrus.Fields{
		"component": "main",
	}).Info("Database migrations completed")

	// Initialize protocol registry
	protocolRegistry := protocol.NewRegistry()
	config.SetProtocolValidator(protocolRegistry)

	// Register protocol modules
	if err := protocolRegistry.Register(protocol.NewEthereumModule()); err != nil {
		log.WithFields(logrus.Fields{
			"component": "main",
			"error":     err.Error(),
		}).Error("Failed to register Ethereum protocol module")
		return 1
	}

	if err := protocolRegistry.Register(protocol.NewArbitrumModule()); err != nil {
		log.WithFields(logrus.Fields{
			"component": "main",
			"error":     err.Error(),
		}).Error("Failed to register Arbitrum protocol module")
		return 1
	}

	log.WithFields(logrus.Fields{
		"component": "main",
		"protocols": protocolRegistry.List(),
	}).Info("Protocol modules registered")

	// Initialize notification registry
	notificationRegistry := notification.NewRegistry()
	config.SetNotificationValidator(notificationRegistry)

	// Register notification modules
	if err := notificationRegistry.Register(notification.NewDiscordModule()); err != nil {
		log.WithFields(logrus.Fields{
			"component": "main",
			"error":     err.Error(),
		}).Error("Failed to register Discord notification module")
		return 1
	}

	log.WithFields(logrus.Fields{
		"component": "main",
		"types":     notificationRegistry.List(),
	}).Info("Notification modules registered")

	// Initialize command executor
	exec := executor.NewDefaultExecutor(log.Logger)

	// Initialize upload manager with database adapter
	dbAdapter := &DatabaseAdapter{db: db}
	uploadMgr := upload.NewManager(exec, dbAdapter, log.Logger)

	// Initialize scheduler
	sched := scheduler.NewCronScheduler(log.Logger)

	// Add global status update job (upload monitor)
	monitorJob := scheduler.NewUploadMonitorJob(uploadMgr, db, cfg.Nodes, log.Logger)
	if err := sched.AddJob(cfg.Schedule, monitorJob); err != nil {
		log.WithFields(logrus.Fields{
			"component": "main",
			"error":     err.Error(),
			"schedule":  cfg.Schedule,
		}).Error("Failed to add upload monitor job")
		return 1
	}

	log.WithFields(logrus.Fields{
		"component": "main",
		"schedule":  cfg.Schedule,
	}).Info("Upload monitor job scheduled")

	// Add per-node upload jobs
	for nodeName, nodeConfig := range cfg.Nodes {
		nodeSchedule := cfg.GetNodeSchedule(nodeName)
		nodeNotifications := cfg.GetNodeNotifications(nodeName)

		uploadJob := scheduler.NewNodeUploadJob(
			nodeName,
			nodeConfig,
			protocolRegistry,
			uploadMgr,
			db,
			notificationRegistry,
			nodeNotifications,
			log.Logger,
		)

		if err := sched.AddJob(nodeSchedule, uploadJob); err != nil {
			log.WithFields(logrus.Fields{
				"component": "main",
				"node":      nodeName,
				"error":     err.Error(),
				"schedule":  nodeSchedule,
			}).Error("Failed to add node upload job")
			return 1
		}

		log.WithFields(logrus.Fields{
			"component": "main",
			"node":      nodeName,
			"schedule":  nodeSchedule,
		}).Info("Node upload job scheduled")
	}

	// Start the scheduler
	sched.Start()

	log.WithFields(logrus.Fields{
		"component": "main",
	}).Info("Scheduler started, daemon is now running")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Wait for shutdown signal
	sig := <-sigChan
	log.WithFields(logrus.Fields{
		"component": "main",
		"signal":    sig.String(),
	}).Info("Received shutdown signal, initiating graceful shutdown")

	// Cancel context to signal all goroutines to stop
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Use WaitGroup to track shutdown completion
	var wg sync.WaitGroup

	// Stop scheduler
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := sched.Stop(shutdownCtx); err != nil {
			log.WithFields(logrus.Fields{
				"component": "main",
				"error":     err.Error(),
			}).Warn("Scheduler shutdown timeout")
		}
	}()

	// Wait for all shutdown tasks to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.WithFields(logrus.Fields{
			"component": "main",
		}).Info("Graceful shutdown completed")
		return 0
	case <-shutdownCtx.Done():
		log.WithFields(logrus.Fields{
			"component": "main",
		}).Error("Shutdown timeout exceeded, forcing exit")
		return 1
	}
}

// handleStatusCommand handles the 'snapperd status' subcommand
func handleStatusCommand(configPath string, consoleMode bool) int {
	// Initialize logger
	log := logger.New(logger.Config{
		Level:       "info",
		ConsoleMode: consoleMode,
	})

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "status",
			"error":     err.Error(),
		}).Error("Failed to load configuration")
		return 1
	}

	// Connect to database
	ctx := context.Background()
	dbCfg := database.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		Database: cfg.Database.Database,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		SSLMode:  cfg.Database.SSLMode,
	}

	db, err := database.New(ctx, dbCfg)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "status",
			"error":     err.Error(),
		}).Error("Failed to connect to database")
		return 1
	}
	defer db.Close()

	// Get running uploads
	runningUploads, err := db.GetRunningUploads(ctx)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "status",
			"error":     err.Error(),
		}).Error("Failed to get running uploads")
		return 1
	}

	// Display results
	if len(runningUploads) == 0 {
		fmt.Println("No active uploads")
		return 0
	}

	fmt.Printf("Active uploads: %d\n\n", len(runningUploads))
	for _, upload := range runningUploads {
		fmt.Printf("Node: %s (%s)\n", upload.NodeName, upload.Protocol)
		fmt.Printf("  Upload ID: %d\n", upload.ID)
		fmt.Printf("  Started: %s\n", upload.StartedAt.Format(time.RFC3339))
		fmt.Printf("  Duration: %s\n", time.Since(upload.StartedAt).Round(time.Second))
		fmt.Printf("  Trigger: %s\n", upload.TriggerType)

		// Display protocol data (blockchain state when upload started)
		if upload.ProtocolData != nil {
			fmt.Printf("  Blockchain State:\n")
			if latestBlock, ok := upload.ProtocolData["latest_block"]; ok && latestBlock != nil {
				fmt.Printf("    Latest Block: %v\n", latestBlock)
			}
			if latestSlot, ok := upload.ProtocolData["latest_slot"]; ok && latestSlot != nil {
				fmt.Printf("    Latest Slot: %v\n", latestSlot)
			}
			if earliestBlob, ok := upload.ProtocolData["earliest_blob"]; ok && earliestBlob != nil {
				fmt.Printf("    Earliest Blob: %v\n", earliestBlob)
			}
		}

		// Note: Progress data is now stored separately in upload_progress table
		// For real-time progress, we'd need to query that table
		fmt.Printf("  Status: %s\n", upload.Status)
		fmt.Println()
	}

	return 0
}

// handleUploadCommand handles the 'snapperd upload <node>' subcommand
func handleUploadCommand(configPath string, consoleMode bool, nodeName string) int {
	// Initialize logger
	log := logger.New(logger.Config{
		Level:       "info",
		ConsoleMode: consoleMode,
	})

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "upload",
			"error":     err.Error(),
		}).Error("Failed to load configuration")
		return 1
	}

	// Verify node exists in configuration
	nodeConfig, exists := cfg.Nodes[nodeName]
	if !exists {
		fmt.Fprintf(os.Stderr, "Error: node '%s' not found in configuration\n", nodeName)
		return 1
	}

	// Connect to database
	ctx := context.Background()
	dbCfg := database.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		Database: cfg.Database.Database,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		SSLMode:  cfg.Database.SSLMode,
	}

	db, err := database.New(ctx, dbCfg)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "upload",
			"error":     err.Error(),
		}).Error("Failed to connect to database")
		return 1
	}
	defer db.Close()

	// Initialize protocol registry
	protocolRegistry := protocol.NewRegistry()
	if err := protocolRegistry.Register(protocol.NewEthereumModule()); err != nil {
		log.WithFields(logrus.Fields{
			"component": "upload",
			"error":     err.Error(),
		}).Error("Failed to register Ethereum protocol module")
		return 1
	}
	if err := protocolRegistry.Register(protocol.NewArbitrumModule()); err != nil {
		log.WithFields(logrus.Fields{
			"component": "upload",
			"error":     err.Error(),
		}).Error("Failed to register Arbitrum protocol module")
		return 1
	}

	// Initialize notification registry
	notificationRegistry := notification.NewRegistry()
	if err := notificationRegistry.Register(notification.NewDiscordModule()); err != nil {
		log.WithFields(logrus.Fields{
			"component": "upload",
			"error":     err.Error(),
		}).Error("Failed to register Discord notification module")
		return 1
	}

	// Initialize command executor and upload manager
	exec := executor.NewDefaultExecutor(log.Logger)
	dbAdapter := &DatabaseAdapter{db: db}
	uploadMgr := upload.NewManager(exec, dbAdapter, log.Logger)

	// Check if upload is already running (checks both database and actual command status)
	shouldSkip, err := uploadMgr.ShouldSkipUpload(ctx, nodeName)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"error":     err.Error(),
		}).Error("Failed to check for running upload")
		return 1
	}

	if shouldSkip {
		fmt.Fprintf(os.Stderr, "Error: upload already running for node '%s'\n", nodeName)
		return 1
	}

	// Execute the upload workflow
	fmt.Printf("Starting manual upload for node '%s'...\n", nodeName)

	// Step 1: Collect metrics
	protocolModule, err := protocolRegistry.Get(nodeConfig.Protocol)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"error":     err.Error(),
		}).Error("Failed to get protocol module")
		return 1
	}

	metrics, err := protocolModule.CollectMetrics(ctx, nodeConfig)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"error":     err.Error(),
		}).Warn("Failed to collect metrics, continuing with partial data")
		metrics = map[string]interface{}{
			"error": err.Error(),
		}
	}

	fmt.Println("Metrics collected")

	// Step 2: Initiate upload with protocol data
	uploadID, err := uploadMgr.InitiateUploadWithProtocolData(ctx, nodeName, "manual", nodeConfig.Protocol, nodeConfig.Type, metrics)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": "upload",
			"node":      nodeName,
			"error":     err.Error(),
		}).Error("Failed to initiate upload")
		return 1
	}

	fmt.Printf("Upload initiated successfully (ID: %d)\n", uploadID)

	// Send notification if configured
	nodeNotifications := cfg.GetNodeNotifications(nodeName)
	if nodeNotifications != nil && nodeNotifications.Complete {
		payload := notification.NotificationPayload{
			Event:     notification.EventComplete,
			NodeName:  nodeName,
			Timestamp: time.Now(),
			Message:   "Manual upload initiated",
			Details: map[string]interface{}{
				"upload_id":    uploadID,
				"trigger_type": "manual",
			},
		}

		// Send to all configured notification types
		for notificationType := range nodeNotifications.Types {
			notifyModule, err := notificationRegistry.Get(notificationType)
			if err != nil {
				continue
			}

			url := nodeNotifications.GetNotificationURL(notificationType)
			if url != "" {
				_ = notifyModule.Send(ctx, url, payload)
			}
		}
	}

	return 0
}
