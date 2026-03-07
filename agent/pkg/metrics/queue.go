package metrics

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const defaultQueueDir = "/var/lib/proxera/metrics_queue"

// Queue provides persistent on-disk buffering for metric buckets
// that couldn't be sent to the backend (e.g. during disconnects).
// Each batch is stored as a separate JSON file named by nanosecond timestamp.
type Queue struct {
	dir string
}

// NewQueue creates a new persistent metrics queue.
func NewQueue() *Queue {
	return &Queue{dir: defaultQueueDir}
}

// Enqueue persists a batch of buckets to a timestamped file on disk.
func (q *Queue) Enqueue(buckets []MetricsBucket) error {
	if len(buckets) == 0 {
		return nil
	}

	if err := os.MkdirAll(q.dir, 0755); err != nil {
		return fmt.Errorf("create queue dir: %w", err)
	}

	data, err := json.Marshal(buckets)
	if err != nil {
		return fmt.Errorf("marshal buckets: %w", err)
	}

	filename := fmt.Sprintf("%d.json", time.Now().UnixNano())
	path := filepath.Join(q.dir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write queue file: %w", err)
	}

	log.Printf("[metrics-queue] enqueued %d bucket(s) to %s", len(buckets), filename)
	return nil
}

// Dequeue reads and removes the oldest batch from the queue.
// Returns nil, nil if the queue is empty.
func (q *Queue) Dequeue() ([]MetricsBucket, error) {
	files, err := q.listFiles()
	if err != nil || len(files) == 0 {
		return nil, err
	}

	path := files[0]
	data, err := os.ReadFile(path)
	if err != nil {
		os.Remove(path) // unreadable, discard
		return nil, fmt.Errorf("read queue file %s: %w", filepath.Base(path), err)
	}

	var buckets []MetricsBucket
	if err := json.Unmarshal(data, &buckets); err != nil {
		os.Remove(path) // corrupted, discard
		return nil, fmt.Errorf("unmarshal queue file %s: %w", filepath.Base(path), err)
	}

	os.Remove(path)
	return buckets, nil
}

// HasBacklog returns true if there are queued files waiting to be sent.
func (q *Queue) HasBacklog() bool {
	files, _ := q.listFiles()
	return len(files) > 0
}

// BacklogSize returns the number of queued files.
func (q *Queue) BacklogSize() int {
	files, _ := q.listFiles()
	return len(files)
}

func (q *Queue) listFiles() ([]string, error) {
	pattern := filepath.Join(q.dir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(files) // sorted by timestamp (filename is unix nanos)
	return files, nil
}
