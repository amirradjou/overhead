package live

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// As Open-Meteo answered for Toronto's harbour on 2026-10-30.
const openMeteo = `{"latitude":43.646603,"longitude":-79.38272,"current_units":{"time":"iso8601"},"current":{"time":"2026-10-30T19:00","interval":900,"temperature_2m":13.8,"weather_code":3,"cloud_cover":100,"cloud_cover_low":100,"cloud_cover_mid":0,"cloud_cover_high":0,"visibility":37000.0,"wind_speed_10m":18.2,"wind_direction_10m":300,"wind_gusts_10m":29.2}}`

func TestWeather(t *testing.T) {
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.RequestURI()
		_, _ = w.Write([]byte(openMeteo))
	}))
	defer srv.Close()
	c := &WeatherClient{Base: srv.URL, HTTP: srv.Client(), UserAgent: "test"}
	w, err := c.Now(context.Background(), pearson)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asked, "latitude=43.678") || !strings.Contains(asked, "cloud_cover_low") {
		t.Errorf("asked %s", asked)
	}
	want := Weather{At: time.Date(2026, 10, 30, 19, 0, 0, 0, time.UTC), TempC: 13.8, Code: 3, CloudCover: 100,
		CloudLow: 100, VisibilityM: 37000, WindKmh: 18.2, WindFromDeg: 300, GustKmh: 29.2}
	if w != want {
		t.Fatalf("weather = %+v", w)
	}

	s := &State{Center: pearson, RadiusKm: 30}
	s.SetWeather(w)
	if v := s.View(w.At.Add(30 * time.Minute)); v.Weather == nil || v.Weather.WindFromDeg != 300 {
		t.Fatalf("fresh weather not in the view: %+v", v.Weather)
	}
	if v := s.View(w.At.Add(2 * time.Hour)); v.Weather != nil {
		t.Fatal("a two-hour-old reading is still shown")
	}
}

func TestWeatherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := &WeatherClient{Base: srv.URL, HTTP: srv.Client(), UserAgent: "test"}
	if _, err := c.Now(context.Background(), pearson); err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v", err)
	}
}
