// Package bike builds a road-bike ride time model from a BRouter route.
//
// BRouter supplies geometry, elevation, road types and traffic signals. Ride time
// comes from a simple physics model calibrated to the rider's speed on the flat,
// with speed caps on slow road types, so the climb home is slower than the way in.
package bike

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const brouterURL = "https://brouter.de/brouter"

// Model describes rider and bike.
type Model struct {
	FlatKmh       float64 // cruising speed on the flat, no wind
	MaxKmh        float64 // descents and fast roads
	MassKg        float64 // rider + bike
	CdA           float64 // m², road bike on the hoods ~0.32
	Crr           float64 // rolling resistance, road tyres ~0.005
	ClimbPowerX   float64 // riders push harder uphill
	SignalPenalty time.Duration
}

func DefaultModel() Model {
	return Model{FlatKmh: 30, MaxKmh: 45, MassKg: 85, CdA: 0.32, Crr: 0.005, ClimbPowerX: 1.25, SignalPenalty: 10 * time.Second}
}

// Point is a route vertex with cumulative distance and modelled ride time.
type Point struct {
	Lat, Lon, Ele float64
	DistM         float64
	TimeS         float64
}

// Route is a parsed and timed ride.
type Route struct {
	DistanceM float64
	AscentM   float64
	DescentM  float64
	Signals   int
	RideS     float64 // moving time incl. signal penalties
	Points    []Point
}

// Fetch asks brouter.de for a route between two coordinates.
func Fetch(ctx context.Context, hc *http.Client, profile string, fromLat, fromLon, toLat, toLon float64) ([]byte, error) {
	v := url.Values{}
	v.Set("lonlats", fmt.Sprintf("%.6f,%.6f|%.6f,%.6f", fromLon, fromLat, toLon, toLat))
	v.Set("profile", profile)
	v.Set("alternativeidx", "0")
	v.Set("format", "geojson")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, brouterURL+"?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "commute-selfhosted/0.1")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(body)), "{") {
		return nil, fmt.Errorf("brouter: HTTP %d: %.200s", resp.StatusCode, body)
	}
	return body, nil
}

type rawRoute struct {
	Features []struct {
		Properties struct {
			Messages [][]string `json:"messages"`
		} `json:"properties"`
		Geometry struct {
			Coordinates [][]float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

// speed caps in km/h by OSM highway type: places where nobody rides at 30.
var capsKmh = map[string]float64{
	"corridor":      6,
	"steps":         4,
	"pedestrian":    12,
	"living_street": 15,
	"footway":       16,
	"path":          20,
	"service":       20,
	"track":         22,
	"residential":   25,
	"cycleway":      28,
}

// Parse reads a BRouter GeoJSON route and times it with m.
func Parse(body []byte, m Model) (*Route, error) {
	var r rawRoute
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("brouter: decode: %w", err)
	}
	if len(r.Features) == 0 || len(r.Features[0].Geometry.Coordinates) < 2 {
		return nil, fmt.Errorf("brouter: empty route")
	}
	f := r.Features[0]
	pts := make([]Point, len(f.Geometry.Coordinates))
	for i, c := range f.Geometry.Coordinates {
		if len(c) < 2 {
			return nil, fmt.Errorf("brouter: bad coordinate")
		}
		pts[i] = Point{Lon: c[0], Lat: c[1]}
		if len(c) > 2 {
			pts[i].Ele = c[2]
		} else if i > 0 {
			pts[i].Ele = pts[i-1].Ele
		}
		if i > 0 {
			pts[i].DistM = pts[i-1].DistM + haversine(pts[i-1].Lat, pts[i-1].Lon, pts[i].Lat, pts[i].Lon)
		}
	}

	// Per-segment speed caps and signals from the message table. Each message row
	// ends at a coordinate and describes the way leading up to it.
	caps := make([]float64, len(pts)) // caps[i] applies to segment (i-1, i]
	signals := 0
	if msgs := f.Properties.Messages; len(msgs) > 1 {
		col := map[string]int{}
		for i, h := range msgs[0] {
			col[h] = i
		}
		index := map[[2]int64]int{}
		for i, p := range pts {
			index[[2]int64{int64(math.Round(p.Lon * 1e6)), int64(math.Round(p.Lat * 1e6))}] = i
		}
		prev := 0
		for _, row := range msgs[1:] {
			lon, _ := strconv.ParseInt(field(row, col, "Longitude"), 10, 64)
			lat, _ := strconv.ParseInt(field(row, col, "Latitude"), 10, 64)
			end, ok := index[[2]int64{lon, lat}]
			if !ok || end <= prev {
				continue
			}
			c := capFor(field(row, col, "WayTags"))
			for i := prev + 1; i <= end; i++ {
				caps[i] = c
			}
			if nt := field(row, col, "NodeTags"); strings.Contains(nt, "highway=traffic_signals") || strings.Contains(nt, "crossing=traffic_signals") {
				signals++
			}
			prev = end
		}
	}

	ele := smooth(pts, 100)
	route := &Route{DistanceM: pts[len(pts)-1].DistM, Signals: signals, Points: pts}
	for i := 1; i < len(pts); i++ {
		d := ele[i] - ele[i-1]
		if d > 0 {
			route.AscentM += d
		} else {
			route.DescentM -= d
		}
	}

	power := m.flatPower()
	t := 0.0
	for i := 1; i < len(pts); i++ {
		dist := pts[i].DistM - pts[i-1].DistM
		if dist <= 0 {
			pts[i].TimeS = t
			continue
		}
		grade := math.Max(-0.15, math.Min(0.15, (ele[i]-ele[i-1])/dist))
		p := power
		if grade > 0.02 {
			p *= m.ClimbPowerX
		}
		v := m.speed(p, grade)
		if caps[i] > 0 {
			v = math.Min(v, caps[i]/3.6)
		}
		t += dist / v
		pts[i].TimeS = t
	}
	// Spread signal waits evenly over the route so positions along it stay plausible.
	extra := float64(signals) * m.SignalPenalty.Seconds()
	if t > 0 && extra > 0 {
		f := (t + extra) / t
		for i := range pts {
			pts[i].TimeS *= f
		}
		t += extra
	}
	route.RideS = t
	route.AscentM = math.Round(route.AscentM)
	route.DescentM = math.Round(route.DescentM)
	return route, nil
}

func field(row []string, col map[string]int, name string) string {
	if i, ok := col[name]; ok && i < len(row) {
		return row[i]
	}
	return ""
}

func capFor(wayTags string) float64 {
	for _, kv := range strings.Fields(wayTags) {
		if v, ok := strings.CutPrefix(kv, "highway="); ok {
			return capsKmh[v]
		}
	}
	return 0
}

const (
	g   = 9.81
	rho = 1.2
)

func (m Model) resist(v, grade float64) float64 {
	th := math.Atan(grade)
	return m.MassKg*g*(m.Crr*math.Cos(th)+math.Sin(th)) + 0.5*rho*m.CdA*v*v
}

// flatPower is the power that gives FlatKmh on level ground.
func (m Model) flatPower() float64 {
	v := m.FlatKmh / 3.6
	return m.resist(v, 0) * v
}

// speed solves power = resist(v)·v for v (m/s), capped at MaxKmh.
func (m Model) speed(power, grade float64) float64 {
	vmax := m.MaxKmh / 3.6
	if m.resist(vmax, grade)*vmax <= power {
		return vmax // steep descent or tailwind territory: cap
	}
	lo, hi := 0.5, vmax
	for range 50 {
		mid := (lo + hi) / 2
		if m.resist(mid, grade)*mid > power {
			hi = mid
		} else {
			lo = mid
		}
	}
	return lo
}

// smooth averages elevation over ±window metres to drop DEM noise.
func smooth(pts []Point, window float64) []float64 {
	out := make([]float64, len(pts))
	lo, hi := 0, 0
	sum := 0.0
	for i := range pts {
		for hi < len(pts) && pts[hi].DistM <= pts[i].DistM+window {
			sum += pts[hi].Ele
			hi++
		}
		for lo < hi && pts[lo].DistM < pts[i].DistM-window {
			sum -= pts[lo].Ele
			lo++
		}
		out[i] = sum / float64(hi-lo)
	}
	return out
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp, dl := p2-p1, (lon2-lon1)*math.Pi/180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

// At returns the position after s seconds of riding.
func (r *Route) At(s float64) (lat, lon float64) {
	pts := r.Points
	if s <= 0 {
		return pts[0].Lat, pts[0].Lon
	}
	for i := 1; i < len(pts); i++ {
		if pts[i].TimeS >= s {
			a, b := pts[i-1], pts[i]
			f := 0.0
			if b.TimeS > a.TimeS {
				f = (s - a.TimeS) / (b.TimeS - a.TimeS)
			}
			return a.Lat + (b.Lat-a.Lat)*f, a.Lon + (b.Lon-a.Lon)*f
		}
	}
	last := pts[len(pts)-1]
	return last.Lat, last.Lon
}

// Profile returns about n evenly spaced (km, elevation) samples for a chart.
func (r *Route) Profile(n int) [][2]float64 {
	if n < 2 || len(r.Points) < 2 {
		return nil
	}
	out := make([][2]float64, 0, n)
	j := 0
	for k := 0; k < n; k++ {
		d := r.DistanceM * float64(k) / float64(n-1)
		for j < len(r.Points)-1 && r.Points[j+1].DistM < d {
			j++
		}
		e := r.Points[j].Ele
		if j < len(r.Points)-1 {
			a, b := r.Points[j], r.Points[j+1]
			if b.DistM > a.DistM {
				e = a.Ele + (b.Ele-a.Ele)*(d-a.DistM)/(b.DistM-a.DistM)
			}
		}
		out = append(out, [2]float64{math.Round(d/100) / 10, math.Round(e)})
	}
	return out
}
