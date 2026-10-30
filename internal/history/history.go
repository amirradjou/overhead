// Package history turns the history queue into one-minute batch files of gzipped JSONL (README: History), and checks
// the files for lost or duplicated events.
package history

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/amirradjou/overhead/internal/events"
)

// Store keeps batch files under keys such as positions/dt=2026-10-29/hour=22/....jsonl.gz.
type Store interface {
	Put(ctx context.Context, key string, body []byte) error
}

// DirStore keeps batches in a local directory, the stand-in for S3 until M3.
type DirStore struct{ Root string }

// Put writes body to key, atomically: a reader sees the old file or the new one, never half of one.
func (d DirStore) Put(_ context.Context, key string, body []byte) error {
	path := filepath.Join(d.Root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Key names a batch. Hive-style dt=/hour= directories let DuckDB and Snowflake prune by time; the run and first
// sequence number make the key the same when the same messages are redelivered, so a retry overwrites its own file.
func Key(minute time.Time, run string, firstSeq uint64) string {
	m := minute.UTC()
	return fmt.Sprintf("positions/dt=%s/hour=%02d/%s_%s_%012d.jsonl.gz",
		m.Format(time.DateOnly), m.Hour(), m.Format("20060102T1504Z"), run, firstSeq)
}

// Batch is one minute of one run's positions.
type Batch struct {
	Minute   time.Time
	Run      string
	FirstSeq uint64
	Lines    [][]byte
	// Tags are the deliveries to acknowledge once the batch is stored.
	Tags []uint64
}

// Key is where the batch is stored.
func (b *Batch) Key() string { return Key(b.Minute, b.Run, b.FirstSeq) }

// Encode gzips the batch's JSON lines.
func (b *Batch) Encode() ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	for _, line := range b.Lines {
		if _, err := zw.Write(line); err != nil {
			return nil, err
		}
		if _, err := zw.Write([]byte{'\n'}); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Defaults for Batcher.
const (
	DefaultGrace = 15 * time.Second
	DefaultQuiet = 3 * time.Second
)

// Batcher groups deliveries into batches by the minute the poller published them in.
//
// A batch is closed when a message from a later minute arrives (the queue is in publish order) or, failing that, once
// its minute ended Grace ago and no message arrived for Quiet. The second rule only fires between polls, never while
// a backlog streams in, so the same messages make the same batches when they are redelivered.
type Batcher struct {
	Grace, Quiet time.Duration

	open    []*Batch
	lastAdd time.Time
}

// Add files a delivery and returns the batches it closed.
func (b *Batcher) Add(tag uint64, ev events.Position, line []byte, now time.Time) []*Batch {
	b.lastAdd = now
	minute := ev.Published.UTC().Truncate(time.Minute)
	var closed []*Batch
	var cur *Batch
	b.open = slices.DeleteFunc(b.open, func(bt *Batch) bool {
		if bt.Minute.Before(minute) {
			closed = append(closed, bt)
			return true
		}
		return false
	})
	for _, bt := range b.open {
		if bt.Minute.Equal(minute) && bt.Run == ev.Run {
			cur = bt
		}
	}
	if cur == nil {
		cur = &Batch{Minute: minute, Run: ev.Run, FirstSeq: ev.Seq}
		b.open = append(b.open, cur)
	}
	cur.Lines = append(cur.Lines, line)
	cur.Tags = append(cur.Tags, tag)
	return closed
}

// Due returns the batches the quiet rule closes at now.
func (b *Batcher) Due(now time.Time) []*Batch {
	if now.Sub(b.lastAdd) < b.quiet() {
		return nil
	}
	var due []*Batch
	b.open = slices.DeleteFunc(b.open, func(bt *Batch) bool {
		if now.Sub(bt.Minute.Add(time.Minute)) >= b.grace() {
			due = append(due, bt)
			return true
		}
		return false
	})
	return due
}

// Pending counts the deliveries held in open batches.
func (b *Batcher) Pending() int {
	n := 0
	for _, bt := range b.open {
		n += len(bt.Tags)
	}
	return n
}

func (b *Batcher) grace() time.Duration {
	if b.Grace > 0 {
		return b.Grace
	}
	return DefaultGrace
}

func (b *Batcher) quiet() time.Duration {
	if b.Quiet > 0 {
		return b.Quiet
	}
	return DefaultQuiet
}
