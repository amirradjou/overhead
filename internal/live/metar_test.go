package live

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// As aviationweather.gov answered for Billy Bishop on 2026-10-30, under the broken deck Open-Meteo missed.
const cytzMetar = `[{"icaoId":"CYTZ","receiptTime":"2026-10-30T20:07:24.864Z","obsTime":1793390400,"reportTime":"2026-10-30T20:00:00.000Z","temp":13,"dewp":3,"wdir":300,"wspd":10,"wgst":21,"visib":9,"altim":1024.5,"rawOb":"METAR CYTZ 302000Z AUTO 30010G21KT 9SM BKN055 13/03 A3025 RMK SLP249","cover":"BKN","clouds":[{"cover":"BKN","base":5500}],"fltCat":"VFR"}]`

func metarServer(t *testing.T, body string) *MetarClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ids") != "CYTZ" {
			t.Errorf("asked %s", r.URL.RequestURI())
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &MetarClient{Base: srv.URL, HTTP: srv.Client(), UserAgent: "test"}
}

func TestMetar(t *testing.T) {
	m, err := metarServer(t, cytzMetar).Now(context.Background(), "CYTZ")
	if err != nil {
		t.Fatal(err)
	}
	if m.Station != "CYTZ" || !m.At.Equal(time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)) || m.WindFromDeg == nil ||
		*m.WindFromDeg != 300 || m.WindKt != 10 || m.GustKt != 21 || m.QNH != 1024.5 ||
		len(m.Layers) != 1 || m.Layers[0] != (CloudLayer{"BKN", 5500}) {
		t.Fatalf("metar = %+v", m)
	}
	// Variable wind has no direction.
	vrb := strings.Replace(cytzMetar, `"wdir":300`, `"wdir":"VRB"`, 1)
	if m, err := metarServer(t, vrb).Now(context.Background(), "CYTZ"); err != nil || m.WindFromDeg != nil {
		t.Fatalf("variable wind: %+v, %v", m, err)
	}
}

func TestMetarOverTheModel(t *testing.T) {
	m, err := metarServer(t, cytzMetar).Now(context.Background(), "CYTZ")
	if err != nil {
		t.Fatal(err)
	}
	// Open-Meteo's reading at the same time: nearly clear.
	model := Weather{At: m.At, TempC: 13.7, Code: 0, CloudCover: 15, CloudLow: 15, VisibilityM: 40500, WindKmh: 18.6,
		WindFromDeg: 299, GustKmh: 29.5, PressureHPa: 1024.3}
	s := &State{Center: pearson, RadiusKm: 30}
	s.SetWeather(model)
	s.SetMetar(m)
	v := s.View(m.At.Add(10 * time.Minute))
	w := v.Weather
	if w == nil || w.CloudLow != 75 || w.CloudCover != 75 || w.Code != 2 || w.Station != "CYTZ" || w.PressureHPa != 1024.5 {
		t.Fatalf("weather = %+v", w)
	}
	if w.GustKmh < 38.8 || w.GustKmh > 38.9 || w.WindFromDeg != 300 || w.TempC != 13.7 || w.VisibilityM != 40500 {
		t.Fatalf("wind, temperature or visibility: %+v", w)
	}
	// Overcast with rain is overcast with rain.
	m.Layers, m.Wx = []CloudLayer{{"OVC", 1200}}, "-RA"
	s.SetMetar(m)
	if w := s.View(m.At).Weather; w.Code != 61 || w.CloudLow != 100 {
		t.Fatalf("rain: %+v", w)
	}
	// An old report gives way to the model.
	if w := s.View(m.At.Add(2 * time.Hour)).Weather; w != nil {
		t.Fatalf("stale weather shown: %+v", w)
	}
}

func TestHeightsCorrectedForPressure(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30}
	s.SetWeather(Weather{At: t0, PressureHPa: 1024.5})
	low := pos("aaaaaa", "PTR2537", 43.70, t0)
	high := pos("bbbbbb", "ACA101", 43.72, t0)
	alt := 35000
	high.AltFt = &alt
	s.Update(low)
	s.Update(high)
	byHex := map[string]Plane{}
	for _, p := range s.View(t0).Planes {
		byHex[p.Hex] = p
	}
	// 11.25 hPa above standard: about 307 ft higher than the 3,000 ft reported.
	if p := byHex["aaaaaa"]; p.AltMSLFt == nil || *p.AltMSLFt != 3307 || *p.AltFt != 3000 {
		t.Fatalf("low plane: alt %v, msl %v", *p.AltFt, p.AltMSLFt)
	}
	if p := byHex["bbbbbb"]; p.AltMSLFt != nil {
		t.Fatalf("a flight level was corrected: %v", *p.AltMSLFt)
	}
	if got := pressureFeet(&Weather{PressureHPa: 0}); got != 0 {
		t.Fatalf("unknown pressure corrected by %v", got)
	}
}
