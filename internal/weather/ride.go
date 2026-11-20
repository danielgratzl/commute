package weather

import (
	"strings"
	"time"

	"commute/internal/transit"
)

// Path gives the position after s seconds of riding.
type Path interface {
	At(s float64) (lat, lon float64)
}

const rideStep = 2 * time.Minute

// AssessRide computes the weather for a bike ride leaving the door at leave:
// handling time out, ride along path, handling time in. Every forecast point in d
// can be used along the way (home, work, and any point in between).
func AssessRide(leave time.Time, out, ride, in time.Duration, path Path, d *Data, origin, dest int) *transit.TripWeather {
	if d == nil || origin >= len(d.Points) || dest >= len(d.Points) {
		return nil
	}
	start := leave.Add(out)
	end := start.Add(ride)
	arrive := end.Add(in)
	po, pd := &d.Points[origin], &d.Points[dest]
	if !po.covers(leave) || !pd.covers(arrive) {
		return nil
	}
	w := &transit.TripWeather{Source: d.Source}
	w.Origin = pointWeather(po, "origin", leave)
	w.Dest = pointWeather(pd, "destination", arrive)

	all := make([]int, len(d.Coords))
	for i := range all {
		all[i] = i
	}
	var spans []span
	cur := span{idx: -1}
	for off := time.Duration(0); off < ride; off += rideStep {
		lat, lon := path.At(off.Seconds())
		i := nearestOf(d.Coords, all, lat, lon)
		if i != cur.idx {
			if cur.idx >= 0 {
				cur.iv.End = start.Add(off)
				spans = append(spans, cur)
			}
			cur = span{iv: transit.OutdoorInterval{Kind: "ride", Leg: -1, Start: start.Add(off), Place: placeName(i, origin, dest)}, idx: i}
		}
	}
	if cur.idx >= 0 {
		cur.iv.End = end
		spans = append(spans, cur)
	}
	summarise(w, d, spans)
	w.Verdict, w.Headline, w.Detail = verdict(w)
	w.Detail = strings.Replace(w.Detail, " min outside", " min ride", 1)
	return w
}
