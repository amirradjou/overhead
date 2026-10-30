package enrich

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/amirradjou/overhead/internal/events"
)

// AircraftAPI is what the worker needs from adsbdb.
type AircraftAPI interface {
	Aircraft(ctx context.Context, hex, callsign string) (*events.Aircraft, *events.Route, bool, error)
	Route(ctx context.Context, callsign string) (*events.Route, error)
}

// PhotoAPI is what the worker needs from planespotters.
type PhotoAPI interface {
	Photo(ctx context.Context, hex string) (*events.Photo, error)
}

// Worker answers enrichment jobs from the cache, calling the APIs only for what the cache lacks.
type Worker struct {
	Aircraft AircraftAPI
	Photos   PhotoAPI
	Cache    *Cache

	mu     sync.Mutex
	counts Counts
}

// Counts is the worker's tally; Calls per NewAircraft is M1's "API calls per new plane".
type Counts struct {
	Jobs uint64
	// NewAircraft counts jobs for an airframe the cache did not have.
	NewAircraft uint64
	Calls       uint64
	CacheHits   uint64
	Failures    uint64
}

// Counts returns the tally so far.
func (w *Worker) Counts() Counts {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.counts
}

func (w *Worker) count(f func(*Counts)) {
	w.mu.Lock()
	f(&w.counts)
	w.mu.Unlock()
}

// Enrich looks up the job's airframe, route and photo. The result holds whatever was found even when err is set, so
// a photo that failed doesn't hold back the type and route; retrying the job then costs only the failed call.
func (w *Worker) Enrich(ctx context.Context, job events.EnrichJob) (events.Aircraft, error) {
	w.count(func(c *Counts) { c.Jobs++ })
	out := events.Aircraft{Hex: job.Hex, Looked: w.Cache.now()}

	var ac events.Aircraft
	acHit, acFound, err := w.Cache.get(bucketAircraft, job.Hex, &ac)
	if err != nil {
		return out, err
	}
	var route events.Route
	routeHit, routeFound := false, false
	if job.Callsign != "" {
		if routeHit, routeFound, err = w.Cache.get(bucketRoute, job.Callsign, &route); err != nil {
			return out, err
		}
	}
	w.count(func(c *Counts) {
		c.CacheHits += uint64(btoi(acHit) + btoi(routeHit))
		if !acHit {
			c.NewAircraft++
		}
	})

	var errs []error
	if !acHit {
		callsign := ""
		if job.Callsign != "" && !routeHit {
			callsign = job.Callsign // one call answers both
		}
		a, r, settled, err := w.Aircraft.Aircraft(ctx, job.Hex, callsign)
		w.count(func(c *Counts) { c.Calls++ })
		if err != nil {
			errs = append(errs, err)
		} else {
			acFound = a != nil
			if acFound {
				ac = *a
			}
			if err := put(w.Cache, bucketAircraft, job.Hex, a, ttl(acFound, airframeTTL, airframeMissTTL)); err != nil {
				return out, err
			}
			if settled {
				routeHit, routeFound = true, r != nil
				if routeFound {
					route = *r
				}
				if err := put(w.Cache, bucketRoute, job.Callsign, r, routeTTL); err != nil {
					return out, err
				}
			}
		}
	}
	if job.Callsign != "" && !routeHit && len(errs) == 0 {
		r, err := w.Aircraft.Route(ctx, job.Callsign)
		w.count(func(c *Counts) { c.Calls++ })
		if err != nil {
			errs = append(errs, err)
		} else {
			routeFound = r != nil
			if routeFound {
				route = *r
			}
			if err := put(w.Cache, bucketRoute, job.Callsign, r, routeTTL); err != nil {
				return out, err
			}
		}
	}

	var photo events.Photo
	photoHit, photoFound, err := w.Cache.get(bucketPhoto, job.Hex, &photo)
	if err != nil {
		return out, err
	}
	if photoHit {
		w.count(func(c *Counts) { c.CacheHits++ })
	} else {
		p, err := w.Photos.Photo(ctx, job.Hex)
		w.count(func(c *Counts) { c.Calls++ })
		if err != nil {
			errs = append(errs, err)
		} else {
			photoFound = p != nil
			if photoFound {
				photo = *p
			}
			if err := put(w.Cache, bucketPhoto, job.Hex, p, ttl(photoFound, photoTTL, photoMissTTL)); err != nil {
				return out, err
			}
		}
	}

	if acFound {
		looked := out.Looked
		out = ac
		out.Hex, out.Looked = job.Hex, looked
	}
	if routeFound {
		out.Route = &route
	}
	if photoFound {
		out.Photo = &photo
	}
	err = errors.Join(errs...)
	if err != nil {
		w.count(func(c *Counts) { c.Failures++ })
	}
	return out, err
}

func ttl(found bool, hit, miss time.Duration) time.Duration {
	if found {
		return hit
	}
	return miss
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
