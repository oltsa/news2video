package logger

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// New creates a configured logger.
// It checks APP_ENV for dev mode (pretty printing).
// It defaults log storage to /var/log/app/{serviceName}.log
func New(serviceName string) zerolog.Logger {
	var writers []io.Writer

	// 1. Check Environment
	// If APP_ENV is anything other than "production", we assume Dev mode.
	isDev := os.Getenv("APP_ENV") != "production"

	// 2. Determine Log Directory
	// Default to the Docker volume path, but allow override via env var
	logDir := os.Getenv("LOG_DIR")
	if logDir == "" {
		logDir = "/var/log/app"
	}
	logFilePath := filepath.Join(logDir, serviceName+".log")

	// 3. Configure Console Writer (Stdout)
	if isDev {
		writers = append(writers, zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		})
	} else {
		writers = append(writers, os.Stdout)
	}

	// 4. Configure File Writer (Lumberjack)
	// We attempt to create the directory. If we can't (permissions), we skip file logging
	// so the app doesn't crash, but we print a warning to stdout.
	if err := os.MkdirAll(logDir, 0755); err != nil {
		console := zerolog.ConsoleWriter{Out: os.Stdout}
		console.Write([]byte("WARN: Logger could not create directory " + logDir + ": " + err.Error() + "\n"))
	} else {
		fileLogger := &lumberjack.Logger{
			Filename:   logFilePath,
			MaxSize:    10,   // Megabytes
			MaxBackups: 3,    // Files to keep
			MaxAge:     28,   // Days
			Compress:   true, // Gzip old logs
		}
		writers = append(writers, fileLogger)
	}

	// 5. Combine and Build
	multi := zerolog.MultiLevelWriter(writers...)

	return zerolog.New(multi).
		With().
		Timestamp().
		Str("service", serviceName).
		Logger()
}
