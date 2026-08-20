// Package trajectory appends a task run's typed event stream to JSONL files so
// a run's sequence, timing, and decisions can be replayed and analyzed offline.
// Inspired by Reasonix internal/trajectory.
package trajectory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SchemaVersion identifies the record layout; bump on breaking changes.
const SchemaVersion = 1

// Record is one observed occurrence. Seq orders them and TS is the unix-ms
// observation time at the recorder.
type Record struct {
	SchemaVersion int    `json:"schema_version"`
	TaskID        string `json:"task_id"`
	Seq           uint64 `json:"seq"`
	TS            int64  `json:"ts"`
	EventType     string `json:"event_type"`
	Data          any    `json:"data,omitempty"`
}

// Recorder appends records to a JSONL file.
type Recorder struct {
	mu     sync.Mutex
	f      *os.File
	w      *bufio.Writer
	seq    uint64
	taskID string
	path   string
	closed bool
}

// NewRecorder opens (or creates) a JSONL trajectory file for a task.
func NewRecorder(dir, taskID string) (*Recorder, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create trajectory dir: %w", err)
	}
	safe := sanitize(taskID)
	if safe == "" {
		safe = fmt.Sprintf("task-%d", time.Now().UnixMilli())
	}
	path := filepath.Join(dir, safe+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open trajectory file: %w", err)
	}
	return &Recorder{
		f:      f,
		w:      bufio.NewWriter(f),
		taskID: taskID,
		path:   path,
	}, nil
}

// Append writes one record with an automatically increasing sequence number.
func (r *Recorder) Append(eventType string, data any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("trajectory recorder closed")
	}
	r.seq++
	rec := Record{
		SchemaVersion: SchemaVersion,
		TaskID:        r.taskID,
		Seq:           r.seq,
		TS:            time.Now().UnixMilli(),
		EventType:     eventType,
		Data:          data,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal trajectory record: %w", err)
	}
	if _, err := r.w.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write trajectory record: %w", err)
	}
	if err := r.w.Flush(); err != nil {
		return fmt.Errorf("flush trajectory: %w", err)
	}
	return nil
}

// Path returns the JSONL file path.
func (r *Recorder) Path() string { return r.path }

// Close flushes and closes the file.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if err := r.w.Flush(); err != nil {
		_ = r.f.Close()
		return err
	}
	return r.f.Close()
}

func sanitize(s string) string {
	var out []rune
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
