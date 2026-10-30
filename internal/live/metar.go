package live

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Metar is an airport's weather report, from aviationweather.gov (free, no key): what the observer at the field saw.
// Checked against independent sources on 2026-10-30, Open-Meteo's model had a nearly clear sky under the broken deck
// both Toronto airports reported; the report's cloud layers, gusts and pressure are used over the model's.
type Metar struct {
	Station string
	At      time.Time
	// WindFromDeg is nil when the wind is variable.
	WindFromDeg *float64
	WindKt      float64
	GustKt      float64
	// QNH is the sea-level pressure setting, hPa.
	QNH    float64
	TempC  *float64
	Layers []CloudLayer
	// Wx is present weather: "-RA", "BR", "TSRA".
	Wx string
}

// CloudLayer is one layer of a report: FEW, SCT, BKN, OVC (or VV, the sky hidden), its base in feet above the field.
type CloudLayer struct {
	Cover  string
	BaseFt int
}

// MetarClient asks aviationweather.gov for a station's latest report.
type MetarClient struct {
	Base      string // https://aviationweather.gov in production
	HTTP      *http.Client
	UserAgent string
}

// Now returns the latest report from station (an ICAO code).
func (c *MetarClient) Now(ctx context.Context, station string) (Metar, error) {
	q := url.Values{"ids": {station}, "format": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/api/data/metar?"+q.Encode(), nil)
	if err != nil {
		return Metar{}, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Metar{}, fmt.Errorf("metar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return Metar{}, fmt.Errorf("metar: HTTP %d", resp.StatusCode)
	}
	var reports []struct {
		Station string          `json:"icaoId"`
		Time    string          `json:"reportTime"`
		Temp    *float64        `json:"temp"`
		Wdir    json.RawMessage `json:"wdir"` // degrees, or "VRB"
		Wspd    float64         `json:"wspd"`
		Wgst    float64         `json:"wgst"`
		Altim   float64         `json:"altim"`
		Wx      string          `json:"wxString"`
		Clouds  []struct {
			Cover string `json:"cover"`
			Base  *int   `json:"base"`
		} `json:"clouds"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<18)).Decode(&reports); err != nil {
		return Metar{}, fmt.Errorf("metar: %w", err)
	}
	if len(reports) == 0 {
		return Metar{}, fmt.Errorf("metar: no report for %s", station)
	}
	r := reports[0]
	at, err := time.Parse(time.RFC3339, r.Time)
	if err != nil {
		return Metar{}, fmt.Errorf("metar: time %q: %w", r.Time, err)
	}
	m := Metar{Station: r.Station, At: at, WindKt: r.Wspd, GustKt: r.Wgst, QNH: r.Altim, TempC: r.Temp, Wx: r.Wx}
	var dir float64
	if json.Unmarshal(r.Wdir, &dir) == nil {
		m.WindFromDeg = &dir
	}
	for _, c := range r.Clouds {
		if c.Base != nil {
			m.Layers = append(m.Layers, CloudLayer{Cover: c.Cover, BaseFt: *c.Base})
		}
	}
	return m, nil
}

// coverPercent is how much of the sky a layer covers: FEW 1–2 eighths, SCT 3–4, BKN 5–7, OVC all of it.
var coverPercent = map[string]int{"FEW": 20, "SCT": 45, "BKN": 75, "OVC": 100, "OVX": 100, "VV": 100}

// apply puts the report's sky, wind and pressure over a model's reading. Layers are sorted by height as Open-Meteo
// sorts them: low up to 2,000 m (6,500 ft), mid up to 6,000 m (20,000 ft), high above.
func (m Metar) apply(w Weather) Weather {
	w.CloudLow, w.CloudMid, w.CloudHigh, w.CloudCover = 0, 0, 0, 0
	for _, l := range m.Layers {
		pct := coverPercent[l.Cover]
		switch {
		case l.BaseFt <= 6500:
			w.CloudLow = max(w.CloudLow, pct)
		case l.BaseFt <= 20000:
			w.CloudMid = max(w.CloudMid, pct)
		default:
			w.CloudHigh = max(w.CloudHigh, pct)
		}
		w.CloudCover = max(w.CloudCover, pct)
	}
	// The sky in the WMO code the screen reads: fog, rain, snow and storms from the report's own weather; otherwise
	// clear, mostly clear, partly cloudy or overcast from the cover.
	switch wx := m.Wx; {
	case strings.Contains(wx, "TS"):
		w.Code = 95
	case strings.Contains(wx, "SN"):
		w.Code = 73
	case strings.Contains(wx, "SH") && strings.Contains(wx, "RA"):
		w.Code = 80
	case strings.Contains(wx, "RA"):
		w.Code = 61
	case strings.Contains(wx, "DZ"):
		w.Code = 51
	case strings.Contains(wx, "FG"):
		w.Code = 45
	case w.CloudCover < 10:
		w.Code = 0
	case w.CloudCover < 35:
		w.Code = 1
	case w.CloudCover < 90:
		w.Code = 2
	default:
		w.Code = 3
	}
	if m.WindFromDeg != nil {
		w.WindFromDeg = *m.WindFromDeg
	}
	w.WindKmh = m.WindKt * 1.852
	w.GustKmh = 0
	if m.GustKt > 0 {
		w.GustKmh = m.GustKt * 1.852
	}
	if m.QNH > 900 && m.QNH < 1100 {
		w.PressureHPa = m.QNH
	}
	if w.At.IsZero() && m.TempC != nil {
		w.TempC = *m.TempC
	}
	if m.At.After(w.At) {
		w.At = m.At
	}
	w.Station = m.Station
	return w
}

// SetMetar takes an airport's weather report.
func (s *State) SetMetar(m Metar) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metar = &m
}

// WatchMetar keeps s's airport report fresh: one now, then one every `every`, until ctx ends.
func WatchMetar(ctx context.Context, c *MetarClient, s *State, station string, every time.Duration) {
	for {
		fctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		m, err := c.Now(fctx, station)
		cancel()
		switch {
		case err != nil && ctx.Err() == nil:
			slog.Warn("metar", "station", station, "err", err)
		case err == nil:
			s.SetMetar(m)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// weatherNow is the weather the view shows: the model's reading with the airport's report over it, each only while
// younger than weatherStale; nil when neither is. Caller holds s.mu.
func (s *State) weatherNow(now time.Time) *Weather {
	var w Weather
	have := false
	if s.weather != nil && now.Sub(s.weather.At) <= weatherStale {
		w, have = *s.weather, true
	}
	if s.metar != nil && now.Sub(s.metar.At) <= weatherStale {
		w, have = s.metar.apply(w), true
	}
	if !have {
		return nil
	}
	return &w
}

// pressureFeet is how much higher a plane is than its pressure altitude says: ADS-B reports altitude against the
// standard 1013.25 hPa, so on a high-pressure day a plane is higher than reported (about 300 ft on 2026-10-30, at
// 1024.5 hPa). Above 18,000 ft (Canada's transition altitude) altitudes are flight levels and are left as they are.
func pressureFeet(w *Weather) float64 {
	if w == nil || w.PressureHPa < 900 || w.PressureHPa > 1100 {
		return 0
	}
	return (w.PressureHPa - 1013.25) * 27.3
}

const transitionFt = 18000
