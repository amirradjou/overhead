package history

import (
	"bufio"
	"cmp"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/amirradjou/overhead/internal/events"
)

// RunReport is what the stored history holds of one poller run.
type RunReport struct {
	Run    string
	Events int
	// Last is the highest sequence number stored; the run published at least that many events.
	Last       uint64
	Missing    []Range
	Duplicates int
}

// Range is a span of sequence numbers, both ends included.
type Range struct{ From, To uint64 }

func (r Range) String() string {
	if r.From == r.To {
		return fmt.Sprint(r.From)
	}
	return fmt.Sprintf("%d-%d", r.From, r.To)
}

// MissingCount is how many events in Missing.
func (r RunReport) MissingCount() uint64 {
	var n uint64
	for _, m := range r.Missing {
		n += m.To - m.From + 1
	}
	return n
}

// Verify reads every batch under root and reports, per run, the sequence numbers that are missing (lost) or stored
// more than once (duplicated). Events after the newest stored one can't be judged: they may still be in a batch not
// yet written.
func Verify(root string) ([]RunReport, error) {
	seqs := make(map[string][]uint64)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl.gz") {
			return err
		}
		return readBatch(path, func(run string, seq uint64) { seqs[run] = append(seqs[run], seq) })
	})
	if err != nil {
		return nil, err
	}
	reports := make([]RunReport, 0, len(seqs))
	for run, list := range seqs {
		slices.Sort(list)
		r := RunReport{Run: run, Events: len(list), Last: list[len(list)-1]}
		want := uint64(1)
		for i, s := range list {
			if i > 0 && s == list[i-1] {
				r.Duplicates++
				continue
			}
			if s > want {
				r.Missing = append(r.Missing, Range{want, s - 1})
			}
			want = s + 1
		}
		reports = append(reports, r)
	}
	slices.SortFunc(reports, func(a, b RunReport) int { return strings.Compare(a.Run, b.Run) })
	return reports, nil
}

func readBatch(path string, add func(run string, seq uint64)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var ev struct {
			Run string `json:"run"`
			Seq uint64 `json:"seq"`
		}
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		add(ev.Run, ev.Seq)
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// Read returns the stored events published in [from, to), in publish order (then sequence order), each once.
func Read(root string, from, to time.Time) ([]events.Position, error) {
	var out []events.Position
	seen := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl.gz") {
			return err
		}
		return eachEvent(path, func(ev events.Position) {
			key := fmt.Sprintf("%s/%d", ev.Run, ev.Seq)
			if seen[key] || ev.Published.Before(from) || !ev.Published.Before(to) {
				return
			}
			seen[key] = true
			out = append(out, ev)
		})
	})
	slices.SortFunc(out, func(a, b events.Position) int {
		if c := a.Published.Compare(b.Published); c != 0 {
			return c
		}
		return cmp.Compare(a.Seq, b.Seq)
	})
	return out, err
}

// Each calls fn with every stored event published in [from, to), file by file in time order. Unlike Read it holds
// nothing, so a whole day can be replayed in a small container; whole days outside the range are skipped unread.
// A batch stored twice is seen twice.
func Each(root string, from, to time.Time, fn func(events.Position)) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if day, ok := strings.CutPrefix(d.Name(), "dt="); ok {
				t, perr := time.Parse(time.DateOnly, day) // the batches' UTC day
				if perr == nil && (!t.Add(24*time.Hour).After(from) || !t.Before(to)) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl.gz") {
			return nil
		}
		return eachEvent(path, func(ev events.Position) {
			if !ev.Published.Before(from) && ev.Published.Before(to) {
				fn(ev)
			}
		})
	})
}

func eachEvent(path string, add func(events.Position)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var ev events.Position
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		add(ev)
	}
	return sc.Err()
}
