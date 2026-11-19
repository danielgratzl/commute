package weather

import (
	"fmt"
	"math"
	"sort"
	"time"

	"commute/internal/transit"
)

// Thresholds. The ensemble is hourly and rounded to 0.1 mm, so 0.1 mm/h is "it rains".
const (
	wetRateMMH  = 0.1
	maybeChance = 20 // % from here on: umbrella optional
	wetChance   = 50 // % from here on: take an umbrella
	iceTempC    = 1.0
	iceChance   = 30
	sampleStep  = 5 * time.Minute
)

// Data is the parsed forecast for the configured places (index 0 = home, 1 = work).
type Data struct {
	Points  []Point
	Coords  []Coord
	Source  string
	Fetched time.Time
}

// Attach parses forecast and (optional) ensemble bodies into Data.
func Attach(forecast, ensemble []byte, coords []Coord) (*Data, error) {
	pts, err := ParseForecast(forecast)
	if err != nil {
		return nil, err
	}
	if len(pts) != len(coords) {
		return nil, fmt.Errorf("open-meteo: got %d locations, want %d", len(pts), len(coords))
	}
	d := &Data{Points: pts, Coords: coords, Source: "MeteoSwiss ICON-CH via Open-Meteo"}
	if ensemble != nil {
		times, members, err := ParseEnsemble(ensemble)
		if err == nil && len(times) == len(pts) {
			for i := range d.Points {
				p := &d.Points[i]
				p.Members = members[i]
				// Keep deterministic PoP only if both hourly axes line up.
				if len(p.Hours) != len(times[i]) || len(times[i]) == 0 || !p.Hours[0].Equal(times[i][0]) {
					p.Hours, p.Pop = times[i], nil
				}
			}
			d.Source = "MeteoSwiss ICON-CH1 ensemble via Open-Meteo"
		}
	}
	return d, nil
}

// ---------- point lookups ----------

// periodIndex returns the index of the period (ts[k-1], ts[k]] containing t.
func periodIndex(ts []time.Time, t time.Time) (int, bool) {
	k := sort.Search(len(ts), func(i int) bool { return !ts[i].Before(t) })
	if k == 0 && len(ts) > 0 && ts[0].Equal(t) {
		return 0, true
	}
	if k <= 0 || k >= len(ts) {
		return 0, false
	}
	return k, true
}

func (p *Point) covers(t time.Time) bool {
	_, ok := periodIndex(p.Q15, t)
	return ok
}

// rate is the deterministic precipitation rate in mm/h around t.
func (p *Point) rate(t time.Time) float64 {
	k, ok := periodIndex(p.Q15, t)
	if !ok || k >= len(p.Precip15) {
		return 0
	}
	return p.Precip15[k] * 4
}

func (p *Point) snow(t time.Time) bool {
	k, ok := periodIndex(p.Q15, t)
	if !ok {
		return false
	}
	code := 0
	if k < len(p.Code) {
		code = int(p.Code[k])
	}
	return (k < len(p.Snow15) && p.Snow15[k] > 0) || (code >= 71 && code <= 77) || code == 85 || code == 86
}

// interp linearly interpolates an instantaneous 15-min series at t.
func (p *Point) interp(series []float64, t time.Time) float64 {
	k := sort.Search(len(p.Q15), func(i int) bool { return !p.Q15[i].Before(t) })
	switch {
	case k >= len(p.Q15) || k >= len(series):
		return math.NaN()
	case k == 0 || p.Q15[k].Equal(t):
		return series[k]
	}
	a, b := p.Q15[k-1], p.Q15[k]
	f := float64(t.Sub(a)) / float64(b.Sub(a))
	return series[k-1] + (series[k]-series[k-1])*f
}

// memberRate is member m's precipitation in mm/h for the hour containing t.
func (p *Point) memberRate(m int, t time.Time) (float64, bool) {
	k, ok := periodIndex(p.Hours, t)
	if !ok || k >= len(p.Members[m]) {
		return 0, false
	}
	return p.Members[m][k], true
}

// chance is the % chance of rain in the hour containing t.
func (p *Point) chance(t time.Time) int {
	if len(p.Members) > 0 {
		wet, n := 0, 0
		for m := range p.Members {
			if r, ok := p.memberRate(m, t); ok {
				n++
				if r >= wetRateMMH {
					wet++
				}
			}
		}
		if n > 0 {
			return int(math.Round(100 * float64(wet) / float64(n)))
		}
	}
	if k, ok := periodIndex(p.Hours, t); ok && k < len(p.Pop) {
		return int(p.Pop[k])
	}
	return 0
}

func round1(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Round(v*10) / 10
}

// ---------- trip assessment ----------

// Assess computes the weather for c. origin/dest are indexes into d.Points.
// It returns nil when the forecast does not cover the trip.
func Assess(c transit.Connection, d *Data, origin, dest int) *transit.TripWeather {
	if d == nil || origin >= len(d.Points) || dest >= len(d.Points) {
		return nil
	}
	po, pd := &d.Points[origin], &d.Points[dest]
	if !po.covers(c.DoorDepart) || !pd.covers(c.DoorArrive) {
		return nil
	}
	placeName := func(i int) string {
		if i == origin {
			return "origin"
		}
		return "destination"
	}

	w := &transit.TripWeather{Source: d.Source}
	w.Origin = pointWeather(po, "origin", c.DoorDepart)
	w.Dest = pointWeather(pd, "destination", c.DoorArrive)

	type span struct {
		iv  transit.OutdoorInterval
		idx int // point index
	}
	var spans []span
	for _, iv := range outdoorIntervals(c) {
		lat, lon := iv.lat, iv.lon
		idx := dest
		if lat == 0 && lon == 0 {
			if iv.Leg == 0 {
				idx = origin
			}
		} else {
			idx = nearest(d.Coords, lat, lon)
		}
		iv.Place = placeName(idx)
		spans = append(spans, span{iv.OutdoorInterval, idx})
	}

	nMembers := len(po.Members)
	if len(pd.Members) != nMembers {
		nMembers = 0
	}
	memberWet := make([]bool, nMembers)
	memberPeak := make([]float64, nMembers)
	detPeak, popMax := 0.0, 0
	snow := false

	for si := range spans {
		s := &spans[si]
		p := &d.Points[s.idx]
		w.OutdoorMin += int(s.iv.End.Sub(s.iv.Start) / time.Minute)
		samples := sampleTimes(s.iv.Start, s.iv.End)
		ivWet := 0
		var ivRates []float64
		for m := 0; m < nMembers; m++ {
			peak := 0.0
			for _, t := range samples {
				if r, ok := p.memberRate(m, t); ok && r > peak {
					peak = r
				}
			}
			if peak >= wetRateMMH {
				ivWet++
				ivRates = append(ivRates, peak)
				memberWet[m] = true
			}
			memberPeak[m] = math.Max(memberPeak[m], peak)
		}
		ivDet, ivPop := 0.0, 0
		for _, t := range samples {
			ivDet = math.Max(ivDet, p.rate(t))
			ivPop = max(ivPop, p.chance(t))
			snow = snow || p.snow(t)
		}
		detPeak = math.Max(detPeak, ivDet)
		popMax = max(popMax, ivPop)
		if nMembers > 0 {
			s.iv.Chance = int(math.Round(100 * float64(ivWet) / float64(nMembers)))
			s.iv.RateMMH = round1(median(ivRates))
		} else {
			s.iv.Chance = ivPop
			s.iv.RateMMH = round1(ivDet)
		}
		w.Intervals = append(w.Intervals, s.iv)
	}

	if nMembers > 0 {
		wet := 0
		var rates []float64
		for m := range memberWet {
			if memberWet[m] {
				wet++
				rates = append(rates, memberPeak[m])
			}
		}
		w.Chance = int(math.Round(100 * float64(wet) / float64(nMembers)))
		w.RateMMH = round1(median(rates))
	} else {
		w.Chance = popMax
		w.RateMMH = round1(detPeak)
	}
	if w.OutdoorMin == 0 {
		w.Chance, w.RateMMH = 0, 0
	}
	w.Snow = snow || w.Origin.Snow || w.Dest.Snow
	minTemp := math.Min(w.Origin.Temp, w.Dest.Temp)
	w.Ice = minTemp <= iceTempC && w.Chance >= iceChance
	w.Intensity = intensity(w.Chance, w.RateMMH)
	w.Verdict, w.Headline, w.Detail = verdict(w)
	return w
}

func pointWeather(p *Point, place string, t time.Time) transit.PointWeather {
	return transit.PointWeather{
		Place:   place,
		At:      t,
		Temp:    round1(p.interp(p.Temp, t)),
		Feels:   round1(p.interp(p.Feels, t)),
		RateMMH: round1(p.rate(t)),
		Chance:  p.chance(t),
		Snow:    p.snow(t),
	}
}

type ivWithCoord struct {
	transit.OutdoorInterval
	lat, lon float64
}

// outdoorIntervals lists walks and waits at stops, with realtime delays applied.
func outdoorIntervals(c transit.Connection) []ivWithCoord {
	var out []ivWithCoord
	last := len(c.Legs) - 1
	var prevEnd time.Time
	for i, l := range c.Legs {
		if l.Mode == transit.Walk {
			start := l.Depart
			if i == 0 {
				start = c.DoorDepart
			} else if !prevEnd.IsZero() {
				start = prevEnd
			}
			end := start.Add(time.Duration(l.Minutes) * time.Minute)
			if i == last {
				end = c.DoorArrive
				start = end.Add(-time.Duration(l.Minutes) * time.Minute)
			}
			lat, lon := midpoint(l.FromLat, l.FromLon, l.ToLat, l.ToLon)
			if end.After(start) {
				out = append(out, ivWithCoord{transit.OutdoorInterval{Kind: "walk", Leg: i, Start: start, End: end}, lat, lon})
			}
			prevEnd = end
			continue
		}
		dep := l.Depart.Add(delay(l.DepDelay))
		if i > 0 && !prevEnd.IsZero() && dep.Sub(prevEnd) >= time.Minute {
			out = append(out, ivWithCoord{transit.OutdoorInterval{Kind: "wait", Leg: i - 1, Start: prevEnd, End: dep}, l.FromLat, l.FromLon})
		}
		prevEnd = l.Arrive.Add(delay(l.ArrDelay))
	}
	return out
}

func delay(p *int) time.Duration {
	if p == nil || *p < 0 {
		return 0
	}
	return time.Duration(*p) * time.Minute
}

func midpoint(aLat, aLon, bLat, bLon float64) (float64, float64) {
	switch {
	case aLat == 0 && aLon == 0:
		return bLat, bLon
	case bLat == 0 && bLon == 0:
		return aLat, aLon
	}
	return (aLat + bLat) / 2, (aLon + bLon) / 2
}

func nearest(cs []Coord, lat, lon float64) int {
	best, bestD := 0, math.Inf(1)
	k := math.Cos(lat * math.Pi / 180)
	for i, c := range cs {
		dx, dy := (c.Lon-lon)*k, c.Lat-lat
		if d := dx*dx + dy*dy; d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

func sampleTimes(a, b time.Time) []time.Time {
	var ts []time.Time
	for t := a.Add(time.Second); t.Before(b); t = t.Add(sampleStep) {
		ts = append(ts, t)
	}
	return append(ts, b)
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func intensity(chance int, rate float64) string {
	switch {
	case chance == 0 || rate < wetRateMMH:
		return "none"
	case rate < 1:
		return "light"
	case rate < 4:
		return "moderate"
	default:
		return "heavy"
	}
}

func verdict(w *transit.TripWeather) (v, headline, detail string) {
	what := "rain"
	if w.Snow {
		what = "snow"
	}
	switch {
	case w.OutdoorMin == 0 || w.Chance < maybeChance:
		v, headline = "dry", "No umbrella needed"
	case w.Chance < wetChance:
		v, headline = "maybe", "Umbrella optional"
	default:
		v, headline = "wet", "Take an umbrella"
	}
	if w.Snow && v != "dry" {
		headline = "Snow possible, dress for it"
	}
	if v == "dry" {
		detail = fmt.Sprintf("Dry during your %d min outside", w.OutdoorMin)
		if w.Chance > 0 {
			detail = fmt.Sprintf("%d%% chance of %s during your %d min outside", w.Chance, what, w.OutdoorMin)
		}
	} else {
		adj := w.Intensity
		if adj == "none" {
			adj = "light"
		}
		detail = fmt.Sprintf("%d%% chance of %s %s during your %d min outside", w.Chance, adj, what, w.OutdoorMin)
	}
	if w.Ice {
		detail += " · slippery"
	}
	return
}
