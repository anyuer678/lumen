package service

import (
	"strings"
	"sync"

	"agent/internal/trajectory"
)

// trajManager lazily opens a trajectory recorder per task and closes it when
// the task reaches a terminal state, so every task's run is captured to JSONL.
type trajManager struct {
	mu  sync.Mutex
	m   map[string]*trajectory.Recorder
	dir string
}

func newTrajManager(dir string) *trajManager {
	return &trajManager{m: make(map[string]*trajectory.Recorder), dir: dir}
}

// recorder returns the recorder for a task, opening it on first use.
func (t *trajManager) recorder(taskID string) *trajectory.Recorder {
	t.mu.Lock()
	defer t.mu.Unlock()
	if r, ok := t.m[taskID]; ok {
		return r
	}
	r, err := trajectory.NewRecorder(t.dir, taskID)
	if err != nil {
		return nil
	}
	t.m[taskID] = r
	return r
}

// finalize writes nothing but closes the task's recorder and drops it.
func (t *trajManager) finalize(taskID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if r, ok := t.m[taskID]; ok {
		_ = r.Close()
		delete(t.m, taskID)
	}
}

// isTerminal reports whether an event type marks a run's end.
func isTerminalEvent(eventType string) bool {
	return strings.HasPrefix(eventType, "task.completed") ||
		strings.HasPrefix(eventType, "task.failed") ||
		strings.HasPrefix(eventType, "task.cancelled") ||
		eventType == "task.stopped"
}
