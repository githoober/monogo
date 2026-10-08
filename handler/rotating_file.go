package handler

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
)

// RotatingFileWriter is an io.WriteCloser that writes to a file and rotates it
// based on file size, backup count, and optional gzip compression or daily rollover.
type RotatingFileWriter struct {
	mu         sync.Mutex
	filename   string
	maxSize    int64
	maxBackups int
	maxAgeDays int
	compress   bool
	daily      bool

	file     *os.File
	size     int64
	openDate string
}

var _ io.WriteCloser = (*RotatingFileWriter)(nil)
var _ monogo.Resettable = (*RotatingFileWriter)(nil)

// NewRotatingFileWriter creates a new RotatingFileWriter.
func NewRotatingFileWriter(filename string, opts ...Option) *RotatingFileWriter {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	return &RotatingFileWriter{
		filename:   filename,
		maxSize:    o.maxSize,
		maxBackups: o.maxBackups,
		maxAgeDays: o.maxAgeDays,
		compress:   o.compress,
		daily:      o.daily,
	}
}

// Filename returns the current target log filename.
func (w *RotatingFileWriter) Filename() string {
	return w.filename
}

// Rotate forces a rotation of the current log file.
func (w *RotatingFileWriter) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rotateLocked()
}

// Write writes log bytes to the file, triggering rotation if the size limit is exceeded.
func (w *RotatingFileWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	writeLen := int64(len(p))

	if w.file == nil {
		if err := w.openFile(); err != nil {
			return 0, err
		}
	}

	today := time.Now().Format("2006-01-02")
	if w.daily && w.openDate != "" && w.openDate != today {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	} else if w.maxSize > 0 && w.size+writeLen > w.maxSize && w.size > 0 {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}

	n, err = w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingFileWriter) openFile() error {
	dir := filepath.Dir(w.filename)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(w.filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}

	w.file = f
	w.size = fi.Size()
	w.openDate = time.Now().Format("2006-01-02")

	if w.maxAgeDays > 0 {
		ext := filepath.Ext(w.filename)
		prefix := strings.TrimSuffix(w.filename, ext)
		w.cleanOldBackups(prefix)
	}

	return nil
}

func (w *RotatingFileWriter) backupPath(prefix, ext string, index int) string {
	if ext != "" {
		return fmt.Sprintf("%s.%d%s", prefix, index, ext)
	}
	return fmt.Sprintf("%s.%d", prefix, index)
}

func (w *RotatingFileWriter) rotateLocked() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}

	if fi, err := os.Stat(w.filename); err == nil && fi.Size() > 0 {
		ext := filepath.Ext(w.filename)
		prefix := strings.TrimSuffix(w.filename, ext)

		if w.maxBackups > 0 {
			for i := w.maxBackups; i >= 1; i-- {
				src := w.backupPath(prefix, ext, i)
				if i == w.maxBackups {
					_ = os.Remove(src)
					_ = os.Remove(src + ".gz")
				} else {
					dst := w.backupPath(prefix, ext, i+1)
					if _, err := os.Stat(src + ".gz"); err == nil {
						_ = os.Rename(src+".gz", dst+".gz")
					} else if _, err := os.Stat(src); err == nil {
						if w.compress {
							_ = os.Rename(src, dst+".gz")
						} else {
							_ = os.Rename(src, dst)
						}
					}
				}
			}
		}

		target1 := w.backupPath(prefix, ext, 1)
		if w.compress {
			targetGz := target1 + ".gz"
			if err := compressFile(w.filename, targetGz); err != nil {
				_ = os.Rename(w.filename, target1)
			} else {
				_ = os.Remove(w.filename)
			}
		} else {
			_ = os.Rename(w.filename, target1)
		}

		if w.maxAgeDays > 0 {
			w.cleanOldBackups(prefix)
		}
	}

	return w.openFile()
}

func (w *RotatingFileWriter) cleanOldBackups(prefix string) {
	dir := filepath.Dir(w.filename)
	cutoff := time.Now().Add(-time.Duration(w.maxAgeDays) * 24 * time.Hour)
	basePrefix := filepath.Base(prefix)
	activeName := filepath.Base(w.filename)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == activeName {
			continue
		}
		if strings.HasPrefix(name, basePrefix+".") {
			fullPath := filepath.Join(dir, name)
			if fi, err := os.Stat(fullPath); err == nil {
				if fi.ModTime().Before(cutoff) {
					_ = os.Remove(fullPath)
				}
			}
		}
	}
}

func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		_ = gz.Close()
		return err
	}
	return gz.Close()
}

// Close closes the underlying open file.
func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

// Sync commits the current contents of the file to stable storage.
func (w *RotatingFileWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		return w.file.Sync()
	}
	return nil
}

// Reset commits buffered data and flushes the file.
func (w *RotatingFileWriter) Reset(ctx context.Context) error {
	return w.Sync()
}

// RotatingFile writes log records to a rotating file.
type RotatingFile struct {
	*Stream
	writer *RotatingFileWriter
}

// Writer returns the underlying RotatingFileWriter.
func (r *RotatingFile) Writer() *RotatingFileWriter {
	return r.writer
}

// Rotate forces a rotation of the underlying file.
func (r *RotatingFile) Rotate() error {
	if r.writer != nil {
		return r.writer.Rotate()
	}
	return nil
}

// Sync commits the current contents of the file to stable storage.
func (r *RotatingFile) Sync() error {
	if r.writer != nil {
		return r.writer.Sync()
	}
	return nil
}

// Reset resets per-handler processors and flushes the rotating file writer.
func (r *RotatingFile) Reset(ctx context.Context) error {
	err := r.Stream.Reset(ctx)
	if r.writer != nil {
		if wErr := r.writer.Reset(ctx); wErr != nil && err == nil {
			err = wErr
		}
	}
	return err
}

// NewRotatingFile creates a handler that writes logs to a file with rotation.
// It defaults to LineFormatter unless configured with WithFormatter.
func NewRotatingFile(filename string, level monogo.Level, opts ...Option) *RotatingFile {
	rw := NewRotatingFileWriter(filename, opts...)
	stream := NewStream(rw, level, opts...)
	return &RotatingFile{
		Stream: stream,
		writer: rw,
	}
}

// RotatingJSONFile writes JSON-formatted log records to a file with rotation.
type RotatingJSONFile struct {
	*RotatingFile
}

// NewRotatingJSONFile creates a rotating file handler pre-configured with JSON formatting.
// It writes structured JSON records (NDJSON or JSON array) and rotates files based on size/backups.
func NewRotatingJSONFile(filename string, level monogo.Level, opts ...Option) *RotatingJSONFile {
	jsonOpts := append([]Option{WithFormatter(formatter.NewJSON(""))}, opts...)
	return &RotatingJSONFile{
		RotatingFile: NewRotatingFile(filename, level, jsonOpts...),
	}
}

// NewJSONRotatingFile is an alias for NewRotatingJSONFile.
func NewJSONRotatingFile(filename string, level monogo.Level, opts ...Option) *RotatingJSONFile {
	return NewRotatingJSONFile(filename, level, opts...)
}
