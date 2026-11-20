package bike

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"commute/internal/config"
	"commute/internal/transit"
)

// line builds a straight north-bound BRouter-style route of n segments of segM metres,
// climbing climbPerSeg metres per segment, with the given highway tag and signal count.
func line(t *testing.T, n int, segM, climbPerSeg float64, highway string, signals int) []byte {
	t.Helper()
	const lat0, lon0 = 47.30, 8.55
	dLat := segM / 111195.0
	var coords [][]float64
	msgs := [][]string{{"Longitude", "Latitude", "Elevation", "Distance", "WayTags", "NodeTags"}}
	for i := 0; i <= n; i++ {
		lat := lat0 + float64(i)*dLat
		coords = append(coords, []float64{lon0, lat, 400 + float64(i)*climbPerSeg})
		if i > 0 {
			node := ""
			if i <= signals {
				node = "highway=traffic_signals"
			}
			msgs = append(msgs, []string{
				itoa(math.Round(lon0 * 1e6)), itoa(math.Round(lat * 1e6)), "0", "0", "highway=" + highway + " surface=asphalt", node,
			})
		}
	}
	b, _ := json.Marshal(map[string]any{"features": []any{map[string]any{
		"properties": map[string]any{"messages": msgs},
		"geometry":   map[string]any{"coordinates": coords},
	}}})
	return b
}

func itoa(v float64) string { return json.Number(formatInt(int64(v))).String() }

func formatInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestFlatRideMatchesCruisingSpeed(t *testing.T) {
	r, err := Parse(line(t, 100, 100, 0, "secondary", 0), DefaultModel()) // 10 km flat
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(r.DistanceM-10000) > 20 {
		t.Fatalf("distance %.0f", r.DistanceM)
	}
	if got := r.RideS / 60; math.Abs(got-20) > 0.3 { // 10 km at 30 km/h
		t.Errorf("flat ride %.1f min, want 20", got)
	}
	if r.AscentM != 0 {
		t.Errorf("ascent %.0f", r.AscentM)
	}
}

func TestClimbIsSlowerAndDescentCapped(t *testing.T) {
	m := DefaultModel()
	up, _ := Parse(line(t, 30, 100, 5, "secondary", 0), m)    // 3 km at 5%
	down, _ := Parse(line(t, 30, 100, -5, "secondary", 0), m) // 3 km at -5%
	flat, _ := Parse(line(t, 30, 100, 0, "secondary", 0), m)
	if !(up.RideS > 1.8*flat.RideS) {
		t.Errorf("5%% climb %.1f min vs flat %.1f min: not slow enough", up.RideS/60, flat.RideS/60)
	}
	if down.RideS < 3000/(m.MaxKmh/3.6)-1 {
		t.Errorf("descent faster than the %v km/h cap", m.MaxKmh)
	}
	if math.Abs(up.AscentM-150) > 10 {
		t.Errorf("ascent %.0f, want ~150", up.AscentM)
	}
}

func TestCapsAndSignals(t *testing.T) {
	m := DefaultModel()
	r, _ := Parse(line(t, 20, 100, 0, "living_street", 5), m) // 2 km capped at 15 km/h
	want := 2.0/15*3600 + 5*m.SignalPenalty.Seconds()
	if r.Signals != 5 || math.Abs(r.RideS-want) > 5 {
		t.Errorf("signals %d, ride %.0f s, want %.0f s", r.Signals, r.RideS, want)
	}
	// Position half-way through the ride is half-way along the route.
	lat, _ := r.At(r.RideS / 2)
	mid := (r.Points[0].Lat + r.Points[len(r.Points)-1].Lat) / 2
	if math.Abs(lat-mid) > 0.0005 {
		t.Errorf("position at half time %.5f, want ~%.5f", lat, mid)
	}
}

func TestProfile(t *testing.T) {
	r, _ := Parse(line(t, 50, 100, 2, "secondary", 0), DefaultModel())
	p := r.Profile(11)
	if len(p) != 11 || p[0][0] != 0 || p[10][0] != 5 || p[10][1] != 500 {
		t.Errorf("profile %v", p)
	}
}

func opt(v string, hhmm string) Option {
	tm, _ := time.Parse("15:04", hhmm)
	return Option{Verdict: v, LeaveAt: tm, Weather: &transit.TripWeather{}}
}

func TestAdvice(t *testing.T) {
	cases := []struct {
		opts []Option
		best int
		want string
	}{
		{[]Option{opt("ride", "17:00"), opt("ride", "17:10"), opt("train", "17:20")}, 0, "Dry if you leave by 17:10"},
		{[]Option{opt("train", "17:00"), opt("gear", "17:10"), opt("ride", "17:20")}, 0, "Dry if you leave at 17:20 (later)"},
		{[]Option{opt("ride", "07:30"), opt("train", "07:40"), opt("train", "07:50"), opt("ride", "08:00")}, 2, "Dry if you leave at 08:00 (later)"},
		{[]Option{opt("ride", "07:30"), opt("ride", "07:40"), opt("ride", "07:50"), opt("train", "08:00")}, 2, "Dry if you leave between 07:30 and 07:50"},
		{[]Option{opt("train", "17:00"), opt("train", "17:10")}, 0, "No dry departure between 17:00 and 17:10"},
		{[]Option{opt("ride", "17:00"), opt("ride", "17:10")}, 1, "Dry for every departure from 17:00 to 17:10"},
	}
	for _, c := range cases {
		if got := advice(c.opts, c.best); got != c.want {
			t.Errorf("advice = %q, want %q", got, c.want)
		}
	}
}

func TestJudge(t *testing.T) {
	cases := []struct {
		w    *transit.TripWeather
		want string
	}{
		{nil, "unknown"},
		{&transit.TripWeather{Chance: 10}, "ride"},
		{&transit.TripWeather{Chance: 30}, "gear"},
		{&transit.TripWeather{Chance: 60}, "train"},
		{&transit.TripWeather{Chance: 10, Ice: true}, "unsafe"},
	}
	for _, c := range cases {
		if got, _ := judge(c.w); got != c.want {
			t.Errorf("judge(%+v) = %s, want %s", c.w, got, c.want)
		}
	}
}

func TestBuildWithoutWeather(t *testing.T) {
	c := config.Default().Bike
	r, _ := Parse(line(t, 100, 100, 0, "secondary", 0), ModelFrom(c))
	base := time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC)
	v := Build(r, c, base, 0, nil, 0, 1)
	if v.RideMin != 20 || v.DoorMin != 26 || len(v.Options) != 10 || v.Best.Verdict != "unknown" {
		t.Errorf("view %+v", v)
	}
	if !v.Options[9].LeaveAt.Equal(base.Add(90 * time.Minute)) {
		t.Errorf("last option at %v", v.Options[9].LeaveAt)
	}
	c.KCal = 1.1
	if v := Build(r, c, base, 0, nil, 0, 1); v.RideMin != 22 {
		t.Errorf("k_cal 1.1 / best 6: ride %d min, first %v, best %v", v.RideMin, v.Options[0].LeaveAt, v.Best.LeaveAt)
	}
}
