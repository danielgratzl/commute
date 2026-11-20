// Package weather fetches MeteoSwiss model data from Open-Meteo and works out
// rain and temperature for the minutes of a trip spent outside.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	forecastURL = "https://api.open-meteo.com/v1/forecast"
	ensembleURL = "https://ensemble-api.open-meteo.com/v1/ensemble"
	// MeteoSwiss ICON-CH1 (1 km) blended into ICON-CH2. Must be named explicitly,
	// otherwise Open-Meteo serves ICON-D2 for Zurich.
	ForecastModel = "meteoswiss_icon_seamless"
	EnsembleModel = "meteoswiss_icon_ch1_ensemble"
)

type Coord struct{ Lat, Lon float64 }

type Client struct {
	HTTP        *http.Client
	ForecastURL string
	EnsembleURL string
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 15 * time.Second}, ForecastURL: forecastURL, EnsembleURL: ensembleURL}
}

func coords(pts []Coord) (string, string) {
	var la, lo []string
	for _, p := range pts {
		la = append(la, fmt.Sprintf("%.4f", p.Lat))
		lo = append(lo, fmt.Sprintf("%.4f", p.Lon))
	}
	return strings.Join(la, ","), strings.Join(lo, ",")
}

// FetchForecast gets 15-min temperature/precipitation and hourly rain probability.
func (c *Client) FetchForecast(ctx context.Context, pts []Coord) ([]byte, error) {
	la, lo := coords(pts)
	v := url.Values{}
	v.Set("latitude", la)
	v.Set("longitude", lo)
	v.Set("models", ForecastModel)
	v.Set("minutely_15", "temperature_2m,apparent_temperature,precipitation,snowfall,weather_code,wind_gusts_10m")
	v.Set("daily", "sunrise,sunset")
	v.Set("hourly", "precipitation_probability")
	v.Set("timezone", "GMT")
	v.Set("past_days", "1") // so "now" is always covered, whatever the server's day boundary
	v.Set("forecast_days", "2")
	return c.get(ctx, c.ForecastURL+"?"+v.Encode())
}

// FetchEnsemble gets hourly precipitation for each ICON-CH1 ensemble member.
func (c *Client) FetchEnsemble(ctx context.Context, pts []Coord) ([]byte, error) {
	la, lo := coords(pts)
	v := url.Values{}
	v.Set("latitude", la)
	v.Set("longitude", lo)
	v.Set("models", EnsembleModel)
	v.Set("hourly", "precipitation")
	v.Set("timezone", "GMT")
	v.Set("past_days", "1")
	v.Set("forecast_days", "2")
	return c.get(ctx, c.EnsembleURL+"?"+v.Encode())
}

func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ { // Open-Meteo sometimes answers "overloaded"
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(1500 * time.Millisecond):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "commute-selfhosted/0.1")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("open-meteo: HTTP %d: %.200s", resp.StatusCode, body)
			continue
		}
		return body, nil
	}
	return nil, lastErr
}

// ---------- parsed data ----------

// Point holds the series for one location. Precipitation values at time T are the
// amount that fell in the period ending at T (Open-Meteo convention).
type Point struct {
	Q15      []time.Time // 15-min steps
	Temp     []float64
	Feels    []float64
	Precip15 []float64 // mm per 15 min
	Snow15   []float64 // cm per 15 min
	Code     []float64
	Gust     []float64 // km/h

	Sunrise, Sunset []time.Time // per day

	Hours   []time.Time // hourly steps
	Pop     []float64   // % (deterministic)
	Members [][]float64 // [member][hour] mm per hour; empty without ensemble
}

type rawForecast struct {
	Error  bool   `json:"error"`
	Reason string `json:"reason"`
	M15    struct {
		Time     []string   `json:"time"`
		Temp     []*float64 `json:"temperature_2m"`
		Feels    []*float64 `json:"apparent_temperature"`
		Precip   []*float64 `json:"precipitation"`
		Snowfall []*float64 `json:"snowfall"`
		Code     []*float64 `json:"weather_code"`
		Gust     []*float64 `json:"wind_gusts_10m"`
	} `json:"minutely_15"`
	Daily struct {
		Sunrise []string `json:"sunrise"`
		Sunset  []string `json:"sunset"`
	} `json:"daily"`
	Hourly struct {
		Time []string   `json:"time"`
		Pop  []*float64 `json:"precipitation_probability"`
	} `json:"hourly"`
}

// decodeList accepts a single object or a list (one per location).
func decodeList[T any](body []byte) ([]T, error) {
	var list []T
	if err := json.Unmarshal(body, &list); err == nil {
		return list, nil
	}
	var one T
	if err := json.Unmarshal(body, &one); err != nil {
		return nil, err
	}
	return []T{one}, nil
}

// ParseForecast returns one Point per requested location.
func ParseForecast(body []byte) ([]Point, error) {
	raws, err := decodeList[rawForecast](body)
	if err != nil {
		return nil, fmt.Errorf("open-meteo forecast: %w", err)
	}
	out := make([]Point, len(raws))
	for i, r := range raws {
		if r.Error {
			return nil, fmt.Errorf("open-meteo forecast: %s", r.Reason)
		}
		p := &out[i]
		if p.Q15, err = parseTimes(r.M15.Time); err != nil {
			return nil, err
		}
		p.Temp, p.Feels = floatsNaN(r.M15.Temp), floatsNaN(r.M15.Feels)
		p.Precip15, p.Snow15, p.Code = floats(r.M15.Precip), floats(r.M15.Snowfall), floats(r.M15.Code)
		p.Gust = floats(r.M15.Gust)
		if p.Sunrise, err = parseTimes(r.Daily.Sunrise); err != nil {
			return nil, err
		}
		if p.Sunset, err = parseTimes(r.Daily.Sunset); err != nil {
			return nil, err
		}
		if p.Hours, err = parseTimes(r.Hourly.Time); err != nil {
			return nil, err
		}
		p.Pop = floats(r.Hourly.Pop)
	}
	return out, nil
}

// ParseEnsemble returns, per location, the hourly times and one series per member.
func ParseEnsemble(body []byte) ([][]time.Time, [][][]float64, error) {
	type raw struct {
		Error  bool                       `json:"error"`
		Reason string                     `json:"reason"`
		Hourly map[string]json.RawMessage `json:"hourly"`
	}
	raws, err := decodeList[raw](body)
	if err != nil {
		return nil, nil, fmt.Errorf("open-meteo ensemble: %w", err)
	}
	times := make([][]time.Time, len(raws))
	members := make([][][]float64, len(raws))
	for i, r := range raws {
		if r.Error {
			return nil, nil, fmt.Errorf("open-meteo ensemble: %s", r.Reason)
		}
		var ts []string
		if err := json.Unmarshal(r.Hourly["time"], &ts); err != nil {
			return nil, nil, fmt.Errorf("open-meteo ensemble: time: %w", err)
		}
		if times[i], err = parseTimes(ts); err != nil {
			return nil, nil, err
		}
		var keys []string
		for k := range r.Hourly {
			if k == "precipitation" || strings.HasPrefix(k, "precipitation_member") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			var v []*float64
			if err := json.Unmarshal(r.Hourly[k], &v); err != nil {
				return nil, nil, fmt.Errorf("open-meteo ensemble: %s: %w", k, err)
			}
			members[i] = append(members[i], floats(v))
		}
	}
	return times, members, nil
}

func parseTimes(ss []string) ([]time.Time, error) {
	out := make([]time.Time, len(ss))
	for i, s := range ss {
		t, err := time.ParseInLocation("2006-01-02T15:04", s, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("open-meteo: time %q: %w", s, err)
		}
		out[i] = t
	}
	return out, nil
}

// floatsNaN keeps missing values as NaN (0 °C is a real temperature).
func floatsNaN(in []*float64) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = math.NaN()
		if v != nil {
			out[i] = *v
		}
	}
	return out
}

// floats maps nulls to zero; a missing precipitation value never means rain.
func floats(in []*float64) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		if v != nil {
			out[i] = *v
		}
	}
	return out
}
