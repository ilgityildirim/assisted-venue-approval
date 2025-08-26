package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// LogLevel represents different logging levels
type LogLevel int

const (
	LevelTrace LogLevel = iota - 1
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// LogConfig holds logging configuration
type LogConfig struct {
	Level       LogLevel `json:"level"`
	Format      string   `json:"format"`       // "json" or "text"
	Output      string   `json:"output"`       // "stdout", "stderr", or file path
	EnableFile  bool     `json:"enable_file"`  // Enable file logging
	FilePath    string   `json:"file_path"`    // Log file path
	MaxSize     int64    `json:"max_size"`     // Max log file size in MB
	MaxBackups  int      `json:"max_backups"`  // Number of backup files to keep
	MaxAge      int      `json:"max_age"`      // Max age of log files in days
	EnableAsync bool     `json:"enable_async"` // Enable async logging
}

// Logger provides structured logging with context support
type Logger struct {
	config  LogConfig
	slogger *slog.Logger
	file    *os.File
	asyncCh chan LogEntry
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Component string                 `json:"component,omitempty"`
	VenueID   *int64                 `json:"venue_id,omitempty"`
	UserID    *uint                  `json:"user_id,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	Duration  *time.Duration         `json:"duration,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
}

// DefaultLogConfig returns sensible default logging configuration
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Level:       LevelInfo,
		Format:      "json",
		Output:      "stdout",
		EnableFile:  true,
		FilePath:    "/var/log/venue-validation/app.log",
		MaxSize:     100, // 100MB
		MaxBackups:  5,
		MaxAge:      30,
		EnableAsync: true,
	}
}

// NewLogger creates a new structured logger
func NewLogger(config LogConfig) (*Logger, error) {
	ctx, cancel := context.WithCancel(context.Background())

	logger := &Logger{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}

	// Setup output writer
	var writer io.Writer
	switch config.Output {
	case "stdout":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		// File output
		if err := logger.setupFileLogging(); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to setup file logging: %w", err)
		}
		writer = logger.file
	}

	// Create slog logger
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     slog.Level(config.Level),
		AddSource: true,
	}

	if config.Format == "json" {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	logger.slogger = slog.New(handler)

	// Setup async logging if enabled
	if config.EnableAsync {
		logger.asyncCh = make(chan LogEntry, 1000) // Buffer for async logging
		logger.wg.Add(1)
		go logger.asyncWorker()
	}

	return logger, nil
}

// setupFileLogging creates log directory and file
func (l *Logger) setupFileLogging() error {
	if l.config.FilePath == "" {
		return fmt.Errorf("file path is required for file logging")
	}

	// Create log directory if it doesn't exist
	dir := filepath.Dir(l.config.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open log file
	file, err := os.OpenFile(l.config.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.file = file
	return nil
}

// asyncWorker processes log entries asynchronously
func (l *Logger) asyncWorker() {
	defer l.wg.Done()

	for {
		select {
		case entry := <-l.asyncCh:
			l.writeEntry(entry)
		case <-l.ctx.Done():
			// Drain remaining entries
			for {
				select {
				case entry := <-l.asyncCh:
					l.writeEntry(entry)
				default:
					return
				}
			}
		}
	}
}

// writeEntry writes a log entry to the output
func (l *Logger) writeEntry(entry LogEntry) {
	level := slog.Level(levelFromString(entry.Level))

	attrs := []slog.Attr{
		slog.Time("timestamp", entry.Timestamp),
		slog.String("component", entry.Component),
	}

	if entry.VenueID != nil {
		attrs = append(attrs, slog.Int64("venue_id", *entry.VenueID))
	}
	if entry.UserID != nil {
		attrs = append(attrs, slog.Uint64("user_id", uint64(*entry.UserID)))
	}
	if entry.RequestID != "" {
		attrs = append(attrs, slog.String("request_id", entry.RequestID))
	}
	if entry.Duration != nil {
		attrs = append(attrs, slog.Duration("duration", *entry.Duration))
	}
	if entry.Error != "" {
		attrs = append(attrs, slog.String("error", entry.Error))
	}
	if entry.Caller != "" {
		attrs = append(attrs, slog.String("caller", entry.Caller))
	}

	// Add custom fields
	for key, value := range entry.Fields {
		attrs = append(attrs, slog.Any(key, value))
	}

	l.slogger.LogAttrs(context.Background(), level, entry.Message, attrs...)
}

// Close gracefully shuts down the logger
func (l *Logger) Close() error {
	l.cancel()

	if l.config.EnableAsync {
		close(l.asyncCh)
		l.wg.Wait()
	}

	if l.file != nil {
		return l.file.Close()
	}

	return nil
}

// Logging methods with context support

// WithContext returns a logger with context information
func (l *Logger) WithContext(ctx context.Context) *ContextLogger {
	return &ContextLogger{
		logger: l,
		ctx:    ctx,
	}
}

// WithComponent returns a logger with component information
func (l *Logger) WithComponent(component string) *ComponentLogger {
	return &ComponentLogger{
		logger:    l,
		component: component,
	}
}

// ContextLogger provides context-aware logging
type ContextLogger struct {
	logger *Logger
	ctx    context.Context
}

// ComponentLogger provides component-specific logging
type ComponentLogger struct {
	logger    *Logger
	component string
}

// Trace logs at trace level
func (l *Logger) Trace(msg string, fields ...Field) {
	l.log(LevelTrace, msg, "", fields...)
}

// Debug logs at debug level
func (l *Logger) Debug(msg string, fields ...Field) {
	l.log(LevelDebug, msg, "", fields...)
}

// Info logs at info level
func (l *Logger) Info(msg string, fields ...Field) {
	l.log(LevelInfo, msg, "", fields...)
}

// Warn logs at warning level
func (l *Logger) Warn(msg string, fields ...Field) {
	l.log(LevelWarn, msg, "", fields...)
}

// Error logs at error level
func (l *Logger) Error(msg string, err error, fields ...Field) {
	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}
	l.log(LevelError, msg, errorStr, fields...)
}

// Fatal logs at fatal level and exits
func (l *Logger) Fatal(msg string, err error, fields ...Field) {
	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}
	l.log(LevelFatal, msg, errorStr, fields...)
	l.Close()
	os.Exit(1)
}

// ComponentLogger methods
func (cl *ComponentLogger) Trace(msg string, fields ...Field) {
	cl.logger.log(LevelTrace, msg, "", append(fields, String("component", cl.component))...)
}

func (cl *ComponentLogger) Debug(msg string, fields ...Field) {
	cl.logger.log(LevelDebug, msg, "", append(fields, String("component", cl.component))...)
}

func (cl *ComponentLogger) Info(msg string, fields ...Field) {
	cl.logger.log(LevelInfo, msg, "", append(fields, String("component", cl.component))...)
}

func (cl *ComponentLogger) Warn(msg string, fields ...Field) {
	cl.logger.log(LevelWarn, msg, "", append(fields, String("component", cl.component))...)
}

func (cl *ComponentLogger) Error(msg string, err error, fields ...Field) {
	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}
	cl.logger.log(LevelError, msg, errorStr, append(fields, String("component", cl.component))...)
}

func (cl *ComponentLogger) Fatal(msg string, err error, fields ...Field) {
	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}
	cl.logger.log(LevelFatal, msg, errorStr, append(fields, String("component", cl.component))...)
	cl.logger.Close()
	os.Exit(1)
}

// ContextLogger methods
func (cl *ContextLogger) Trace(msg string, fields ...Field) {
	cl.logger.logWithContext(cl.ctx, LevelTrace, msg, "", fields...)
}

func (cl *ContextLogger) Debug(msg string, fields ...Field) {
	cl.logger.logWithContext(cl.ctx, LevelDebug, msg, "", fields...)
}

func (cl *ContextLogger) Info(msg string, fields ...Field) {
	cl.logger.logWithContext(cl.ctx, LevelInfo, msg, "", fields...)
}

func (cl *ContextLogger) Warn(msg string, fields ...Field) {
	cl.logger.logWithContext(cl.ctx, LevelWarn, msg, "", fields...)
}

func (cl *ContextLogger) Error(msg string, err error, fields ...Field) {
	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}
	cl.logger.logWithContext(cl.ctx, LevelError, msg, errorStr, fields...)
}

func (cl *ContextLogger) Fatal(msg string, err error, fields ...Field) {
	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}
	cl.logger.logWithContext(cl.ctx, LevelFatal, msg, errorStr, fields...)
	cl.logger.Close()
	os.Exit(1)
}

// Internal logging methods
func (l *Logger) log(level LogLevel, msg, errorStr string, fields ...Field) {
	l.logWithContext(context.Background(), level, msg, errorStr, fields...)
}

func (l *Logger) logWithContext(ctx context.Context, level LogLevel, msg, errorStr string, fields ...Field) {
	if level < l.config.Level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     levelToString(level),
		Message:   msg,
		Error:     errorStr,
		Fields:    make(map[string]interface{}),
	}

	// Extract context values
	if requestID := ctx.Value("request_id"); requestID != nil {
		if id, ok := requestID.(string); ok {
			entry.RequestID = id
		}
	}

	if venueID := ctx.Value("venue_id"); venueID != nil {
		if id, ok := venueID.(int64); ok {
			entry.VenueID = &id
		}
	}

	if userID := ctx.Value("user_id"); userID != nil {
		if id, ok := userID.(uint); ok {
			entry.UserID = &id
		}
	}

	// Add caller information
	if level >= LevelWarn {
		_, file, line, ok := runtime.Caller(3)
		if ok {
			entry.Caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
		}
	}

	// Process fields
	for _, field := range fields {
		field.AddTo(entry.Fields)
	}

	if l.config.EnableAsync {
		select {
		case l.asyncCh <- entry:
		default:
			// Async buffer full, log synchronously
			l.writeEntry(entry)
		}
	} else {
		l.writeEntry(entry)
	}
}

// Field represents a structured log field
type Field struct {
	Key   string
	Value interface{}
}

// AddTo adds the field to the provided map
func (f Field) AddTo(m map[string]interface{}) {
	m[f.Key] = f.Value
}

// Field constructors
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

func Uint(key string, value uint) Field {
	return Field{Key: key, Value: value}
}

func Float64(key string, value float64) Field {
	return Field{Key: key, Value: value}
}

func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value}
}

func Time(key string, value time.Time) Field {
	return Field{Key: key, Value: value}
}

func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

func Error(err error) Field {
	return Field{Key: "error", Value: err.Error()}
}

// Utility functions
func levelToString(level LogLevel) string {
	switch level {
	case LevelTrace:
		return "TRACE"
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

func levelFromString(level string) LogLevel {
	switch level {
	case "TRACE":
		return LevelTrace
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	default:
		return LevelInfo
	}
}
