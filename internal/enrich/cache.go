package enrich

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Cache lifetimes (README: routes for a day, airframes for a month). "Unknown" answers are cached too, or every poll
// of an aircraft adsbdb doesn't know would ask again.
const (
	airframeTTL     = 30 * 24 * time.Hour
	airframeMissTTL = 7 * 24 * time.Hour
	routeTTL        = 24 * time.Hour
	photoTTL        = 30 * 24 * time.Hour
	photoMissTTL    = 7 * 24 * time.Hour
)

var (
	bucketAircraft = []byte("aircraft")
	bucketRoute    = []byte("route")
	bucketPhoto    = []byte("photo")
)

// Cache keeps API answers in a bbolt file, so they survive restarts. Only one process can open the file.
type Cache struct {
	db  *bolt.DB
	now func() time.Time
}

type entry struct {
	Found   bool            `json:"found"`
	Data    json.RawMessage `json:"data,omitempty"`
	Expires time.Time       `json:"expires"`
}

// OpenCache opens (or creates) the cache file.
func OpenCache(path string) (*Cache, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketAircraft, bucketRoute, bucketPhoto} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Cache{db: db, now: time.Now}, nil
}

// Close closes the file.
func (c *Cache) Close() error { return c.db.Close() }

// get reads key. hit reports a live entry; found whether the API knew the thing (then dst is filled).
func (c *Cache) get(bucket []byte, key string, dst any) (hit, found bool, err error) {
	var e entry
	err = c.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(bucket).Get([]byte(key))
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &e)
	})
	if err != nil || e.Expires.IsZero() || !c.now().Before(e.Expires) {
		return false, false, err
	}
	if e.Found {
		if err := json.Unmarshal(e.Data, dst); err != nil {
			return false, false, err
		}
	}
	return true, e.Found, nil
}

// put stores *v under key for ttl; a nil v records that the API didn't know key. It is generic, not a method on
// `any`, because a nil *Photo in an `any` is not nil: "unknown" would be stored as found and empty.
func put[T any](c *Cache, bucket []byte, key string, v *T, ttl time.Duration) error {
	e := entry{Found: v != nil, Expires: c.now().Add(ttl)}
	if v != nil {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		e.Data = data
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return c.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(bucket).Put([]byte(key), raw) })
}

// Purge deletes expired entries and returns how many it removed.
func (c *Cache) Purge() (int, error) {
	n := 0
	now := c.now()
	err := c.db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bucketAircraft, bucketRoute, bucketPhoto} {
			b := tx.Bucket(name)
			var stale [][]byte
			err := b.ForEach(func(k, v []byte) error {
				var e entry
				if json.Unmarshal(v, &e) != nil || !now.Before(e.Expires) {
					stale = append(stale, append([]byte(nil), k...))
				}
				return nil
			})
			if err != nil {
				return err
			}
			for _, k := range stale {
				if err := b.Delete(k); err != nil {
					return err
				}
			}
			n += len(stale)
		}
		return nil
	})
	return n, err
}
