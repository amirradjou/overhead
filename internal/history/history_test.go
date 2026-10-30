package history

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
)

var t0 = time.Date(2026, 10, 29, 22, 6, 0, 0, time.UTC)

func ev(run string, seq uint64, published time.Time) (events.Position, []byte) {
	e := events.Position{Run: run, Seq: seq, Published: published, Position: feeds.Position{Hex: "abc123", At: published}}
	b, _ := json.Marshal(e)
	return e, b
}

func TestKey(t *testing.T) {
	got := Key(t0.Add(30*time.Second), "20261029T220000Z-abc123", 42)
	want := "positions/dt=2026-10-29/hour=22/20261029T2206Z_20261029T220000Z-abc123_000000000042.jsonl.gz"
	if got != want {
		t.Fatalf("Key = %q\nwant  %q", got, want)
	}
}

func TestBatcherClosesAMinuteWhenTheNextArrives(t *testing.T) {
	b := &Batcher{}
	now := t0.Add(5 * time.Minute)
	for i, at := range []time.Duration{0, 20 * time.Second, 50 * time.Second} {
		e, line := ev("r", uint64(i+1), t0.Add(at))
		if closed := b.Add(uint64(i+1), e, line, now); len(closed) != 0 {
			t.Fatalf("closed %d batches inside one minute", len(closed))
		}
	}
	e, line := ev("r", 4, t0.Add(65*time.Second))
	closed := b.Add(4, e, line, now)
	if len(closed) != 1 || closed[0].FirstSeq != 1 || len(closed[0].Lines) != 3 || closed[0].Tags[2] != 3 {
		t.Fatalf("closed = %+v", closed)
	}
	if b.Pending() != 1 {
		t.Fatalf("Pending = %d, want the new minute's one message", b.Pending())
	}
}

func TestBatcherDueNeedsGraceAndQuiet(t *testing.T) {
	b := &Batcher{Grace: 15 * time.Second, Quiet: 3 * time.Second}
	e, line := ev("r", 1, t0.Add(10*time.Second))
	b.Add(1, e, line, t0.Add(70*time.Second)) // arrives during the next minute

	if due := b.Due(t0.Add(72 * time.Second)); len(due) != 0 {
		t.Fatal("closed before the grace period")
	}
	if due := b.Due(t0.Add(75*time.Second + time.Millisecond)); len(due) != 1 {
		t.Fatal("not closed after grace and quiet")
	}

	// While messages keep arriving (a backlog), the quiet rule never fires, whatever the clock says.
	b2 := &Batcher{}
	e, line = ev("r", 1, t0)
	b2.Add(1, e, line, t0.Add(time.Hour))
	if due := b2.Due(t0.Add(time.Hour + time.Second)); len(due) != 0 {
		t.Fatal("closed a batch while deliveries were still streaming in")
	}
}

func TestTouchHoldsTheQuietRule(t *testing.T) {
	b := &Batcher{Grace: time.Second, Quiet: 3 * time.Second}
	e, line := ev("r", 1, t0)
	b.Add(1, e, line, t0.Add(time.Hour))
	// A slow store just finished: the next tick must not close the batch, its messages may be waiting.
	b.Touch(t0.Add(time.Hour + 10*time.Second))
	if due := b.Due(t0.Add(time.Hour + 11*time.Second)); len(due) != 0 {
		t.Fatal("closed a batch right after a slow store")
	}
	if due := b.Due(t0.Add(time.Hour + 14*time.Second)); len(due) != 1 {
		t.Fatal("not closed once it had been quiet")
	}
}

func TestBatcherKeepsRunsApart(t *testing.T) {
	b := &Batcher{}
	e1, l1 := ev("old", 900, t0.Add(10*time.Second))
	e2, l2 := ev("new", 1, t0.Add(40*time.Second))
	b.Add(1, e1, l1, t0)
	b.Add(2, e2, l2, t0)
	e3, l3 := ev("new", 2, t0.Add(70*time.Second))
	closed := b.Add(3, e3, l3, t0)
	if len(closed) != 2 || closed[0].Run == closed[1].Run {
		t.Fatalf("closed = %+v, want one batch per run", closed)
	}
}

func TestEncodeAndDirStore(t *testing.T) {
	root := t.TempDir()
	bt := &Batch{Minute: t0, Run: "r", FirstSeq: 1}
	for i := range 3 {
		_, line := ev("r", uint64(i+1), t0)
		bt.Lines = append(bt.Lines, line)
	}
	body, err := bt.Encode()
	if err != nil {
		t.Fatal(err)
	}
	store := DirStore{Root: root}
	ctx := context.Background()
	if err := store.Put(ctx, bt.Key(), body); err != nil {
		t.Fatal(err)
	}
	// A redelivery writes the same key again: one file, not two.
	if err := store.Put(ctx, bt.Key(), body); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(root, "positions", "dt=2026-10-29", "hour=22", "*"))
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	f, _ := os.Open(files[0])
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(zr)
	if n := bytes.Count(raw, []byte("\n")); n != 3 {
		t.Fatalf("%d lines, want 3:\n%s", n, raw)
	}
}

func TestDirStoreStaysInsideItsRoot(t *testing.T) {
	store := DirStore{Root: t.TempDir()}
	for _, key := range []string{"../escape.jsonl.gz", "/etc/x.jsonl.gz", "positions/../../escape.jsonl.gz"} {
		if err := store.Put(context.Background(), key, []byte("x")); err == nil {
			t.Errorf("Put(%q) wrote outside the root", key)
		}
	}
}

func TestVerify(t *testing.T) {
	root := t.TempDir()
	store := DirStore{Root: root}
	write := func(run string, minute time.Time, seqs ...uint64) {
		bt := &Batch{Minute: minute, Run: run, FirstSeq: seqs[0]}
		for _, s := range seqs {
			_, line := ev(run, s, minute)
			bt.Lines = append(bt.Lines, line)
		}
		body, _ := bt.Encode()
		if err := store.Put(context.Background(), bt.Key(), body); err != nil {
			t.Fatal(err)
		}
	}
	write("good", t0, 1, 2, 3)
	write("good", t0.Add(time.Minute), 4, 5)
	write("gappy", t0, 1, 2)
	write("gappy", t0.Add(time.Minute), 5, 6, 9)
	write("gappy", t0.Add(2*time.Minute), 6) // a second file holding seq 6 again

	reports, err := Verify(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 {
		t.Fatalf("reports = %+v", reports)
	}
	gappy, good := reports[0], reports[1]
	if good.Run != "good" || good.Events != 5 || good.Last != 5 || len(good.Missing) != 0 || good.Duplicates != 0 {
		t.Errorf("good = %+v", good)
	}
	if gappy.Last != 9 || gappy.Duplicates != 1 || gappy.MissingCount() != 4 ||
		gappy.Missing[0] != (Range{3, 4}) || gappy.Missing[1] != (Range{7, 8}) {
		t.Errorf("gappy = %+v", gappy)
	}
	if s := (Range{7, 8}).String(); s != "7-8" {
		t.Errorf("Range.String = %q", s)
	}
}
