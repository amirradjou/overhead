package live

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/amirradjou/overhead/internal/geo"
)

// Weather is the weather at home now, from Open-Meteo (open-meteo.com: free for non-commercial use, credited on
// screen). The screen draws the clouds it reports and tells the wind, which decides the runway in use.
type Weather struct {
	At    time.Time `json:"at"`
	TempC float64   `json:"temp_c"`
	// Code is the WMO weather code: 0 clear, 1–3 partly cloudy to overcast, 45/48 fog, 51–67 drizzle and rain,
	// 71–77 snow, 80–82 showers, 85/86 snow showers, 95–99 thunderstorms.
	Code int `json:"code"`
	// Cloud cover in percent: all of it, and low (up to about 2 km), mid and high.
	CloudCover  int     `json:"cloud_cover"`
	CloudLow    int     `json:"cloud_low"`
	CloudMid    int     `json:"cloud_mid"`
	CloudHigh   int     `json:"cloud_high"`
	VisibilityM float64 `json:"visibility_m"`
	WindKmh     float64 `json:"wind_kmh"`
	// WindFromDeg is where the wind blows from, degrees clockwise from north.
	WindFromDeg float64 `json:"wind_from_deg"`
	GustKmh     float64 `json:"gust_kmh"`
}

// weatherStale is how old a reading may be and still be shown: past it, the screen draws no clouds rather than old
// ones.
const weatherStale = 90 * time.Minute

// WeatherClient asks Open-Meteo for the weather at a place.
type WeatherClient struct {
	Base      string // https://api.open-meteo.com in production
	HTTP      *http.Client
	UserAgent string
}

// Now returns the current weather at p.
func (c *WeatherClient) Now(ctx context.Context, p geo.Point) (Weather, error) {
	q := url.Values{
		"latitude":  {fmt.Sprintf("%.3f", p.Lat)},
		"longitude": {fmt.Sprintf("%.3f", p.Lon)},
		"current": {"temperature_2m,weather_code,cloud_cover,cloud_cover_low,cloud_cover_mid,cloud_cover_high," +
			"visibility,wind_speed_10m,wind_direction_10m,wind_gusts_10m"},
		"wind_speed_unit": {"kmh"},
		"timezone":        {"UTC"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/v1/forecast?"+q.Encode(), nil)
	if err != nil {
		return Weather{}, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Weather{}, fmt.Errorf("open-meteo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return Weather{}, fmt.Errorf("open-meteo: HTTP %d", resp.StatusCode)
	}
	var ans struct {
		Current struct {
			Time       string  `json:"time"`
			Temp       float64 `json:"temperature_2m"`
			Code       int     `json:"weather_code"`
			Cloud      int     `json:"cloud_cover"`
			Low        int     `json:"cloud_cover_low"`
			Mid        int     `json:"cloud_cover_mid"`
			High       int     `json:"cloud_cover_high"`
			Visibility float64 `json:"visibility"`
			Wind       float64 `json:"wind_speed_10m"`
			WindFrom   float64 `json:"wind_direction_10m"`
			Gust       float64 `json:"wind_gusts_10m"`
		} `json:"current"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&ans); err != nil {
		return Weather{}, fmt.Errorf("open-meteo: %w", err)
	}
	cur := ans.Current
	at, err := time.Parse("2006-01-02T15:04", cur.Time)
	if err != nil {
		return Weather{}, fmt.Errorf("open-meteo: time %q: %w", cur.Time, err)
	}
	return Weather{
		At: at, TempC: cur.Temp, Code: cur.Code, CloudCover: cur.Cloud, CloudLow: cur.Low, CloudMid: cur.Mid,
		CloudHigh: cur.High, VisibilityM: cur.Visibility, WindKmh: cur.Wind, WindFromDeg: cur.WindFrom, GustKmh: cur.Gust,
	}, nil
}

// SetWeather takes a weather reading.
func (s *State) SetWeather(w Weather) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.weather = &w
}

// WatchWeather keeps s's weather fresh: a reading now, then one every `every`, until ctx ends. A failed fetch keeps
// the last reading, which the view drops once it is older than weatherStale.
func WatchWeather(ctx context.Context, c *WeatherClient, s *State, every time.Duration) {
	for {
		fctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		w, err := c.Now(fctx, s.Center)
		cancel()
		switch {
		case err != nil && ctx.Err() == nil:
			slog.Warn("weather", "err", err)
		case err == nil:
			s.SetWeather(w)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}
