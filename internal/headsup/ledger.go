package headsup

import (
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Deduper lets each notice through once.
type Deduper interface {
	// First reports whether n is new, and records it.
	First(n Notice) (bool, error)
}

// learning is how long a new ledger only records types: at first every type is "new", which says nothing.
const learning = 7 * 24 * time.Hour

var (
	bucketSent  = []byte("sent")
	bucketTypes = []byte("types")
	bucketMeta  = []byte("meta")
	keyCreated  = []byte("created")
)

// Ledger keeps sent notices and seen types in a bbolt file, so a restart neither repeats a heads-up nor forgets
// what it has seen.
type Ledger struct {
	db      *bolt.DB
	created time.Time
	now     func() time.Time
}

// OpenLedger opens (or creates) the ledger file.
func OpenLedger(path string) (*Ledger, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
	l := &Ledger{db: db, now: time.Now}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketSent, bucketTypes, bucketMeta} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		meta := tx.Bucket(bucketMeta)
		if raw := meta.Get(keyCreated); raw != nil {
			return l.created.UnmarshalText(raw)
		}
		l.created = l.now()
		raw, _ := l.created.MarshalText()
		return meta.Put(keyCreated, raw)
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return l, nil
}

// Close closes the file.
func (l *Ledger) Close() error { return l.db.Close() }

// First records n and reports whether it was new.
func (l *Ledger) First(n Notice) (bool, error) {
	first := false
	err := l.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSent)
		key := []byte(n.Key())
		if b.Get(key) != nil {
			return nil
		}
		first = true
		return b.Put(key, []byte(n.At.UTC().Format(time.RFC3339)))
	})
	return first, err
}

// Known records the type and reports whether it was seen before. During the first week every type counts as known.
func (l *Ledger) Known(code string) bool {
	known := true
	_ = l.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTypes)
		if b.Get([]byte(code)) == nil {
			known = l.now().Sub(l.created) < learning
			return b.Put([]byte(code), []byte(l.now().UTC().Format(time.RFC3339)))
		}
		return nil
	})
	return known
}

// MemLedger is a Deduper in memory, for replays.
type MemLedger struct {
	mu   sync.Mutex
	sent map[string]bool
}

// First records n and reports whether it was new.
func (m *MemLedger) First(n Notice) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sent == nil {
		m.sent = make(map[string]bool)
	}
	if m.sent[n.Key()] {
		return false, nil
	}
	m.sent[n.Key()] = true
	return true, nil
}
