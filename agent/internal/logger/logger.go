package logger

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

// Logger wraps logrus.Logger with additional functionality
type Logger struct {
	*logrus.Logger
}

// Config holds logger configuration
type Config struct {
	// Level is the log level (debug, info, warn, error)
	Level string
	// ConsoleMode enables human-readable text formatting
	ConsoleMode bool
	// Output is the writer for log output (defaults to os.Stdout)
	Output io.Writer
}

// New creates a new logger with the specified configuration
func New(cfg Config) *Logger {
	log := logrus.New()

	// Set output
	if cfg.Output != nil {
		log.SetOutput(cfg.Output)
	} else {
		log.SetOutput(os.Stdout)
	}

	// Set log level
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		// Default to info level if parsing fails
		level = logrus.InfoLevel
	}
	log.SetLevel(level)

	// Set formatter based on mode
	if cfg.ConsoleMode {
		// Human-readable text format for console mode
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	} else {
		// JSON format for daemon mode (systemd)
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
	}

	return &Logger{Logger: log}
}

// WithComponent returns a logger entry with the component field set
func (l *Logger) WithComponent(component string) *logrus.Entry {
	return l.WithField("component", component)
}

// WithError returns a logger entry with error details
func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err)
}

// WithFields returns a logger entry with multiple fields
func (l *Logger) WithFields(fields logrus.Fields) *logrus.Entry {
	return l.Logger.WithFields(fields)
}

// Default creates a logger with default settings (info level, JSON format)
func Default() *Logger {
	return New(Config{
		Level:       "info",
		ConsoleMode: false,
	})
}

// Console creates a logger for console mode (info level, text format)
func Console() *Logger {
	return New(Config{
		Level:       "info",
		ConsoleMode: true,
	})
}
