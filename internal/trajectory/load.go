package trajectory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// List returns the trajectory files (.jsonl) in a directory, sorted by name.
// Each entry carries the parsed task id (file base name).
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read trajectory dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".jsonl") {
			names = append(names, strings.TrimSuffix(e.Name(), ".jsonl"))
		}
	}
	sort.Strings(names)
	return names, nil
}

// Load reads all records of a trajectory file (best-effort per line).
func Load(dir, taskID string) ([]Record, error) {
	safe := sanitize(taskID)
	path := dir + string(os.PathSeparator) + safe + ".jsonl"
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open trajectory: %w", err)
	}
	defer f.Close()

	var recs []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			continue // 跳过损坏行
		}
		recs = append(recs, r)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan trajectory: %w", err)
	}
	return recs, nil
}
