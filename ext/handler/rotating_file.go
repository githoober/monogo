package handler

import (
	"github.com/githoober/monogo"
	corehandler "github.com/githoober/monogo/handler"
	"gopkg.in/natefinch/lumberjack.v2"
)

// RotatingFileOptions specifies rotation settings for log files.
type RotatingFileOptions struct {
	MaxSizeMB  int  // Max size in MB before rotation (default 100MB)
	MaxBackups int  // Max number of old log files to retain (default 3)
	MaxAgeDays int  // Max number of days to retain old log files
	Compress   bool // Whether to compress rotated log files with gzip
}

// WithRotation configures file rotation using a RotatingFileOptions struct.
func WithRotation(opts RotatingFileOptions) Option {
	return func(o *options) {
		if opts.MaxSizeMB > 0 {
			o.maxSizeMB = opts.MaxSizeMB
		}
		if opts.MaxBackups > 0 {
			o.maxBackups = opts.MaxBackups
		}
		if opts.MaxAgeDays > 0 {
			o.maxAgeDays = opts.MaxAgeDays
		}
		o.compress = opts.Compress
	}
}

// WithMaxSize sets the maximum file size in megabytes before rotation (default 100MB).
func WithMaxSize(maxSizeMB int) Option {
	return func(o *options) {
		o.maxSizeMB = maxSizeMB
	}
}

// WithMaxBackups sets the maximum number of old log files to retain (default 3).
func WithMaxBackups(maxBackups int) Option {
	return func(o *options) {
		o.maxBackups = maxBackups
	}
}

// WithMaxAge sets the maximum age in days before old log files are deleted.
func WithMaxAge(maxAgeDays int) Option {
	return func(o *options) {
		o.maxAgeDays = maxAgeDays
	}
}

// WithCompress sets whether rotated log files should be compressed with gzip.
func WithCompress(compress bool) Option {
	return func(o *options) {
		o.compress = compress
	}
}

// NewRotatingFile creates a Stream handler configured with rolling/rotating file support powered by lumberjack.
func NewRotatingFile(filename string, level monogo.Level, opts ...Option) *corehandler.Stream {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	lj := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    o.maxSizeMB,
		MaxBackups: o.maxBackups,
		MaxAge:     o.maxAgeDays,
		Compress:   o.compress,
	}

	return corehandler.NewStream(lj, level, toCoreOptions(o)...)
}
