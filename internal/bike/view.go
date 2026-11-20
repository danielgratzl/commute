package bike

import (
	"fmt"
	"math"
	"time"

	"commute/internal/config"
	"commute/internal/transit"
	"commute/internal/weather"
)

// ModelFrom turns the config into a ride model.
func ModelFrom(c config.Bike) Model {
	m := DefaultModel()
	m.FlatKmh, m.MaxKmh = c.FlatKmh, c.MaxKmh
	if c.MassKg > 0 {
		m.MassKg = c.MassKg
	}
	m.SignalPenalty = time.Duration(c.SignalPenaltyS) * time.Second
	return m
}

// Option is one possible departure by bike.
type Option struct {
	LeaveAt    time.Time            `json:"leave_at"`
	DoorArrive time.Time            `json:"door_arrive"`
	Verdict    string               `json:"verdict"` // ride | gear | train | unsafe | unknown
	Headline   string               `json:"headline"`
	Weather    *transit.TripWeather `json:"weather"`
}

// View is the bike section of the live response.
type View struct {
	Label      string       `json:"label"`
	DistanceKm float64      `json:"distance_km"`
	AscentM    float64      `json:"ascent_m"`
	DescentM   float64      `json:"descent_m"`
	Signals    int          `json:"signals"`
	RideMin    int          `json:"ride_min"`
	DoorMin    int          `json:"door_min"` // incl. handling at both ends
	Profile    [][2]float64 `json:"profile"`  // [km, m]
	Best       Option       `json:"best"`     // leaving at the suggested time
	BestIndex  int          `json:"best_index"`
	Options    []Option     `json:"options"` // every 10 min around the suggested time
	Advice     string       `json:"advice"`
	Warnings   []string     `json:"warnings,omitempty"`
}

const (
	scanStep  = 10 * time.Minute
	scanCount = 10
)

// Build times the ride for ten departures 10 min apart, with the suggested departure
// base at position bestIdx (0 = scan only later departures), and judges the weather.
// origin/dest index the weather points; wx may be nil.
func Build(r *Route, c config.Bike, base time.Time, bestIdx int, wx *weather.Data, origin, dest int) *View {
	bestIdx = max(0, min(bestIdx, scanCount-1))
	ride := time.Duration(r.RideS*c.KCal) * time.Second
	out := time.Duration(c.HandlingOutMin) * time.Minute
	in := time.Duration(c.HandlingInMin) * time.Minute
	door := out + ride + in

	v := &View{
		Label:      c.Label,
		DistanceKm: math.Round(r.DistanceM/100) / 10,
		AscentM:    r.AscentM,
		DescentM:   r.DescentM,
		Signals:    r.Signals,
		RideMin:    int(ride.Round(time.Minute) / time.Minute),
		DoorMin:    int(door.Round(time.Minute) / time.Minute),
		Profile:    r.Profile(60),
	}
	path := scaledPath{r, c.KCal}
	for k := 0; k < scanCount; k++ {
		leave := base.Add(time.Duration(k-bestIdx) * scanStep)
		o := Option{LeaveAt: leave, DoorArrive: leave.Add(door)}
		o.Weather = weather.AssessRide(leave, out, ride, in, path, wx, origin, dest)
		o.Verdict, o.Headline = judge(o.Weather)
		v.Options = append(v.Options, o)
	}
	v.Best, v.BestIndex = v.Options[bestIdx], bestIdx
	v.Advice = advice(v.Options, bestIdx)
	if w := v.Best.Weather; w != nil {
		if w.Dark {
			v.Warnings = append(v.Warnings, "Dark for part of the ride: lights on")
		}
		if w.GustKmh >= 45 {
			v.Warnings = append(v.Warnings, fmt.Sprintf("Gusts up to %.0f km/h", w.GustKmh))
		}
		if w.Origin.Temp <= 3 || w.Dest.Temp <= 3 {
			v.Warnings = append(v.Warnings, fmt.Sprintf("Cold: %.0f° feels like %.0f°", math.Min(w.Origin.Temp, w.Dest.Temp), math.Min(w.Origin.Feels, w.Dest.Feels)))
		}
	}
	return v
}

// scaledPath maps ride seconds onto the model's timing when k_cal stretches the ride.
type scaledPath struct {
	r *Route
	k float64
}

func (p scaledPath) At(s float64) (float64, float64) { return p.r.At(s / p.k) }

func judge(w *transit.TripWeather) (string, string) {
	switch {
	case w == nil:
		return "unknown", "No forecast for this time"
	case w.Ice || w.Snow:
		return "unsafe", "Slippery, take the train"
	case w.Chance >= 50:
		return "train", "Rain likely, take the train"
	case w.Chance >= 20:
		return "gear", "Rain possible, pack rain gear"
	default:
		return "ride", "Good to ride"
	}
}

// advice describes the dry departures around the suggested one.
func advice(opts []Option, best int) string {
	if len(opts) == 0 || opts[best].Weather == nil {
		return ""
	}
	dry := func(i int) bool { return opts[i].Verdict == "ride" }
	hhmm := func(i int) string { return opts[i].LeaveAt.Format("15:04") }
	last := len(opts) - 1
	if dry(best) {
		a, b := best, best
		for a > 0 && dry(a-1) {
			a--
		}
		for b < last && dry(b+1) {
			b++
		}
		switch {
		case a == 0 && b == last:
			return "Dry for every departure from " + hhmm(0) + " to " + hhmm(last)
		case a == best:
			return "Dry if you leave by " + hhmm(b)
		default:
			return "Dry if you leave between " + hhmm(a) + " and " + hhmm(b)
		}
	}
	for d := 1; d <= last; d++ { // nearest dry departure, earlier ones first on a tie
		for _, i := range []int{best - d, best + d} {
			if i >= 0 && i <= last && dry(i) {
				when := "later"
				if i < best {
					when = "earlier"
				}
				return "Dry if you leave at " + hhmm(i) + " (" + when + ")"
			}
		}
	}
	return "No dry departure between " + hhmm(0) + " and " + hhmm(last)
}
