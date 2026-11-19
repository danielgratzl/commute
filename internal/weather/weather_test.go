package weather

import (
	"os"
	"testing"
	"time"

	"commute/internal/transit"
)

var (
	home = Coord{47.5015, 8.7304} // Kunstmuseum Winterthur (public stand-in)
	work = Coord{47.4083, 8.5067} // ETH Hönggerberg (public stand-in)
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseFixtures(t *testing.T) {
	d, err := Attach(read(t, "openmeteo_forecast.json"), read(t, "openmeteo_ensemble.json"), []Coord{home, work})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Points) != 2 {
		t.Fatalf("points %d", len(d.Points))
	}
	p := d.Points[0]
	if len(p.Q15) != 192 || len(p.Temp) != 192 {
		t.Errorf("15-min series: %d times, %d temps", len(p.Q15), len(p.Temp))
	}
	if len(p.Members) != 11 {
		t.Errorf("members %d, want 11", len(p.Members))
	}
	if d.Source != "MeteoSwiss ICON-CH1 ensemble via Open-Meteo" {
		t.Errorf("source %q", d.Source)
	}
}

func TestAssessRealTrip(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Zurich")
	d, err := Attach(read(t, "openmeteo_forecast.json"), read(t, "openmeteo_ensemble.json"), []Coord{home, work})
	if err != nil {
		t.Fatal(err)
	}
	cs, err := transit.Parse(read(t, "searchch_h2w_tomorrow_arriveby.json"), transit.NormaliseOpts{Loc: loc})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		w := Assess(c, d, 0, 1)
		if w == nil {
			t.Fatalf("%s: no weather", c.DoorDepart)
		}
		if w.OutdoorMin <= 0 || len(w.Intervals) == 0 {
			t.Errorf("%s: outdoor %d min, %d intervals", c.DoorDepart.Format("15:04"), w.OutdoorMin, len(w.Intervals))
		}
		if first := w.Intervals[0]; first.Kind != "walk" || first.Place != "origin" {
			t.Errorf("first interval %+v", first)
		}
		if last := w.Intervals[len(w.Intervals)-1]; last.Place != "destination" {
			t.Errorf("last interval %+v", last)
		}
		if w.Origin.Temp < -30 || w.Origin.Temp > 40 || w.Verdict == "" || w.Headline == "" {
			t.Errorf("odd weather %+v", w)
		}
	}
}

// synthetic builds two points with 15-min and hourly axes around day, raining
// (all members, 2 mm/h) at the destination in the hour ending wetHourEnd.
func synthetic(day time.Time, wetHourEnd time.Time, wetMembers int) *Data {
	mk := func(wet bool) Point {
		var p Point
		for t := day; t.Before(day.Add(48 * time.Hour)); t = t.Add(15 * time.Minute) {
			p.Q15 = append(p.Q15, t)
			p.Temp = append(p.Temp, 0.5)
			p.Feels = append(p.Feels, -2)
			v := 0.0
			if wet && t.After(wetHourEnd.Add(-time.Hour)) && !t.After(wetHourEnd) {
				v = 0.5
			}
			p.Precip15 = append(p.Precip15, v)
			p.Snow15 = append(p.Snow15, 0)
			p.Code = append(p.Code, 3)
		}
		for t := day; t.Before(day.Add(48 * time.Hour)); t = t.Add(time.Hour) {
			p.Hours = append(p.Hours, t)
		}
		p.Members = make([][]float64, 11)
		for m := range p.Members {
			p.Members[m] = make([]float64, len(p.Hours))
			for k, h := range p.Hours {
				if wet && m < wetMembers && h.Equal(wetHourEnd) {
					p.Members[m][k] = 2
				}
			}
		}
		return p
	}
	return &Data{Points: []Point{mk(false), mk(true)}, Coords: []Coord{home, work}, Source: "test"}
}

func trip(t0 time.Time) transit.Connection {
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	ip := func(v int) *int { return &v }
	legs := []transit.Leg{
		{Mode: transit.Walk, Depart: at(0), Arrive: at(5), Minutes: 5, ToLat: 47.4990, ToLon: 8.7290},
		{Mode: transit.Bus, Line: "140", Depart: at(5), Arrive: at(16), FromLat: 47.4990, FromLon: 8.7290, DepDelay: ip(0), ArrDelay: ip(0)},
		{Mode: transit.Train, Line: "S2", Depart: at(22), Arrive: at(35), FromLat: 47.5000, FromLon: 8.7240, DepDelay: ip(0), ArrDelay: ip(0)},
		{Mode: transit.Tram, Line: "8", Depart: at(38), Arrive: at(45), FromLat: 47.4115, FromLon: 8.5440},
		{Mode: transit.Walk, Depart: at(45), Arrive: at(50), Minutes: 5, FromLat: 47.4080, FromLon: 8.5080},
	}
	return transit.Connection{DoorDepart: at(0), DoorArrive: at(50), Legs: legs}
}

func TestAssessWetAtDestination(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, 11, 20, 7, 10, 0, 0, time.UTC)
	d := synthetic(day, time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC), 8) // rain 07:00-08:00 UTC at work, 8 of 11 members
	w := Assess(trip(start), d, 0, 1)
	if w == nil {
		t.Fatal("nil weather")
	}
	// outdoor: walk 5 + wait at Winterthur 6 + wait at Oerlikon 3 + walk 5 = 19 min
	if w.OutdoorMin != 19 {
		t.Errorf("outdoor %d, want 19", w.OutdoorMin)
	}
	if len(w.Intervals) != 4 || w.Intervals[1].Kind != "wait" || w.Intervals[1].Place != "origin" || w.Intervals[2].Place != "destination" {
		t.Errorf("intervals %+v", w.Intervals)
	}
	if w.Chance != 73 || w.Verdict != "wet" || w.Headline != "Take an umbrella" {
		t.Errorf("chance %d verdict %s %q", w.Chance, w.Verdict, w.Headline)
	}
	if w.Intervals[0].Chance != 0 || w.Intervals[3].Chance != 73 {
		t.Errorf("per-interval chances %d / %d", w.Intervals[0].Chance, w.Intervals[3].Chance)
	}
	if w.Intensity != "moderate" || w.RateMMH != 2 {
		t.Errorf("intensity %s %.1f", w.Intensity, w.RateMMH)
	}
	if !w.Ice {
		t.Error("0.5 °C with 73% chance should flag ice")
	}
	if w.Detail != "73% chance of moderate rain during your 19 min outside · slippery" {
		t.Errorf("detail %q", w.Detail)
	}
}

func TestAssessDryAndCoverage(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	d := synthetic(day, time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC), 1) // 1 of 11 members
	w := Assess(trip(time.Date(2026, 11, 20, 7, 10, 0, 0, time.UTC)), d, 0, 1)
	if w.Verdict != "dry" || w.Chance != 9 {
		t.Errorf("verdict %s chance %d", w.Verdict, w.Chance)
	}
	if Assess(trip(time.Date(2026, 11, 23, 7, 0, 0, 0, time.UTC)), d, 0, 1) != nil {
		t.Error("trip beyond the forecast should have no weather")
	}
}
