package handler

import (
	"github.com/githoober/monogo"
	"gopkg.in/natefinch/lumberjack.v2"
)

// RotatingFileOptions specifies rotation settings for log files.
type RotatingFileOptions struct {
	MaxSizeMB  int   // Max size in MB before rotation (default 100MB)
	MaxBackups int   // Max number of old log files to retain (default 3)
	MaxAgeDays int   // Max number of days to retain old log files
	Compress   bool  // Whether to compress rotated log files with gzip
	Bubble     *bool // Whether handler allows bubbling (default true)
}

// NewRotatingFile creates a Stream handler configured with rolling/rotating file support powered by lumberjack.
func NewRotatingFile(filename string, level monolog.Level, opts ...RotatingFileOptions) *Stream {
	var opt RotatingFileOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	if opt.MaxSizeMB <= 0 {
		opt.MaxSizeMB = 100
	}
	if opt.MaxBackups <= 0 {
		opt.MaxBackups = 3
	}

	bubble := true
	if opt.Bubble != nil {
		bubble = *opt.Bubble
	}

	lj := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    opt.MaxSizeMB,
		MaxBackups: opt.MaxBackups,
		MaxAge:     opt.MaxAgeDays,
		Compress:   opt.Compress,
	}

	return NewStream(lj, level, bubble)
}
