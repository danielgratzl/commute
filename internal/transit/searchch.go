package transit

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"commute/internal/config"
)

// SearchCh talks to the search.ch timetable API (no key, ~1000 route searches/day).
type SearchCh struct {
	BaseURL string
	HTTP    *http.Client
}

func NewSearchCh() *SearchCh {
	return &SearchCh{
		BaseURL: "https://search.ch/timetable/api/route.json",
		HTTP:    &http.Client{Timeout: 12 * time.Second},
	}
}

// URL builds the request URL for q. Exported for logging and tests.
func (s *SearchCh) URL(q Query, loc *time.Location) string {
	v := url.Values{}
	v.Set("from", fmt.Sprintf("%.5f,%.5f", q.FromLat, q.FromLon))
	v.Set("to", fmt.Sprintf("%.5f,%.5f", q.ToLat, q.ToLon))
	v.Set("num", strconv.Itoa(q.Num))
	v.Set("show_delays", "1")
	v.Set("show_trackchanges", "1")
	if q.At.IsZero() {
		v.Set("pre", "1") // keep the connection you are about to miss
	} else {
		at := q.At.In(loc)
		v.Set("date", at.Format("2006-01-02"))
		v.Set("time", at.Format("15:04"))
	}
	return s.BaseURL + "?" + v.Encode()
}

// Fetch returns the raw response body.
func (s *SearchCh) Fetch(ctx context.Context, q Query, loc *time.Location) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL(q, loc), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "commute-selfhosted/0.1")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search.ch: HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

// ---------- raw response ----------

// flexString accepts JSON strings, numbers and null.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	*f = flexString(string(b))
	return nil
}

type rawExit struct {
	Arrival  string     `json:"arrival"`
	Name     string     `json:"name"`
	StopID   flexString `json:"stopid"`
	Track    flexString `json:"track"`
	ArrDelay *string    `json:"arr_delay"`
	Lat      float64    `json:"lat"`
	Lon      float64    `json:"lon"`
}

type rawLeg struct {
	Departure   string          `json:"departure"`
	Arrival     string          `json:"arrival"`
	Type        string          `json:"type"`
	Line        string          `json:"line"`
	Terminal    string          `json:"terminal"`
	Name        string          `json:"name"`
	StopID      flexString      `json:"stopid"`
	Track       flexString      `json:"track"`
	DepDelay    *string         `json:"dep_delay"`
	Cancelled   bool            `json:"cancelled"`
	RunningTime float64         `json:"runningtime"`
	FgColor     string          `json:"fgcolor"`
	BgColor     string          `json:"bgcolor"`
	Lat         float64         `json:"lat"`
	Lon         float64         `json:"lon"`
	IsAddress   bool            `json:"isaddress"`
	Exit        *rawExit        `json:"exit"`
	Disruptions json.RawMessage `json:"disruptions"`
}

type rawConnection struct {
	Departure     string          `json:"departure"`
	Arrival       string          `json:"arrival"`
	DepDelay      *string         `json:"dep_delay"`
	ArrDelay      *string         `json:"arr_delay"`
	Infeasibility int             `json:"infeasibility_level"`
	Realtime      bool            `json:"realtime"`
	Disruptions   json.RawMessage `json:"disruptions"`
	Legs          []rawLeg        `json:"legs"`
}

type rawResponse struct {
	Connections []rawConnection `json:"connections"`
	Messages    []string        `json:"messages"`
}

// ---------- normalisation ----------

type NormaliseOpts struct {
	Loc           *time.Location
	BufferMin     int
	WalkOverrides map[string]config.WalkOverride
}

// Parse turns a search.ch route.json body into door-to-door connections.
func Parse(body []byte, o NormaliseOpts) ([]Connection, error) {
	var r rawResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("search.ch: decode: %w", err)
	}
	if len(r.Connections) == 0 && len(r.Messages) > 0 {
		return nil, fmt.Errorf("search.ch: %s", strings.Join(r.Messages, "; "))
	}
	seen := map[string]bool{}
	out := make([]Connection, 0, len(r.Connections))
	for _, rc := range r.Connections {
		c, err := normalise(rc, o)
		if err != nil {
			continue // skip a single malformed connection rather than failing the view
		}
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LeaveAt.Before(out[j].LeaveAt) })
	return out, nil
}

func normalise(rc rawConnection, o NormaliseOpts) (Connection, error) {
	c := Connection{Status: StatusOK, Realtime: rc.Realtime}
	cancelled := false
	disr := map[string]bool{}
	collectDisruptions(rc.Disruptions, disr)

	for _, rl := range rc.Legs {
		if rl.Exit == nil && rl.Type == "" {
			continue // trailing address marker, not a leg
		}
		l, err := normaliseLeg(rl, o.Loc)
		if err != nil {
			return c, err
		}
		if l.Cancelled {
			cancelled = true
		}
		if l.DepDelay != nil || l.ArrDelay != nil {
			c.HasRealtime = true
		}
		collectDisruptions(rl.Disruptions, disr)
		c.Legs = append(c.Legs, l)
	}
	if len(c.Legs) == 0 {
		return c, fmt.Errorf("no legs")
	}
	if isX(rc.ArrDelay) || isX(rc.DepDelay) {
		cancelled = true
	}

	applyWalkOverrides(c.Legs, o.WalkOverrides)

	first, last := firstVehicle(c.Legs), lastVehicle(c.Legs)
	c.DoorDepart = c.Legs[0].Depart
	c.DoorArrive = c.Legs[len(c.Legs)-1].Arrive
	c.PlannedArrive = c.DoorArrive
	if first >= 0 {
		if d := deref(c.Legs[first].DepDelay); d > 0 {
			c.DoorDepart = c.DoorDepart.Add(time.Duration(d) * time.Minute) // you can leave a bit later
		}
	}
	if last >= 0 {
		if d := deref(c.Legs[last].ArrDelay); d != 0 {
			c.DoorArrive = c.DoorArrive.Add(time.Duration(d) * time.Minute)
		}
	}
	c.LeaveAt = c.DoorDepart.Add(-time.Duration(o.BufferMin) * time.Minute)
	c.Minutes = int(c.DoorArrive.Sub(c.DoorDepart).Round(time.Minute) / time.Minute)
	c.DelayMin = int(c.DoorArrive.Sub(c.PlannedArrive) / time.Minute)
	if n := countVehicles(c.Legs); n > 0 {
		c.Transfers = n - 1
	}

	switch {
	case cancelled:
		c.Status, c.StatusNote = StatusCancelled, cancelledNote(c.Legs)
	case rc.Infeasibility >= 1:
		c.Status, c.StatusNote = StatusAtRisk, "Transfer at risk because of a delay"
	default:
		if note := transferRisk(c.Legs); note != "" {
			c.Status, c.StatusNote = StatusAtRisk, note
		}
	}

	for s := range disr {
		c.Disruptions = append(c.Disruptions, s)
	}
	sort.Strings(c.Disruptions)
	c.ID = connectionID(c)
	return c, nil
}

func normaliseLeg(rl rawLeg, loc *time.Location) (Leg, error) {
	l := Leg{
		Mode:       mapMode(rl.Type),
		Line:       strings.TrimSpace(rl.Line),
		Direction:  rl.Terminal,
		From:       cleanName(rl.Name),
		FromStopID: string(rl.StopID),
		FgColor:    rl.FgColor,
		BgColor:    rl.BgColor,
		FromLat:    rl.Lat,
		FromLon:    rl.Lon,
		Cancelled:  rl.Cancelled,
	}
	dep, err := parseTime(rl.Departure, loc)
	if err != nil {
		return l, err
	}
	l.Depart = dep
	if rl.Exit != nil {
		l.To = cleanName(rl.Exit.Name)
		l.ToStopID = string(rl.Exit.StopID)
		l.ToLat, l.ToLon = rl.Exit.Lat, rl.Exit.Lon
		arr, err := parseTime(rl.Exit.Arrival, loc)
		if err != nil {
			return l, err
		}
		l.Arrive = arr
		if l.Mode != Walk {
			d, x := parseDelay(rl.Exit.ArrDelay)
			l.ArrDelay = d
			l.Cancelled = l.Cancelled || x
		}
	} else {
		l.Arrive = dep.Add(time.Duration(rl.RunningTime) * time.Second)
	}
	if l.Mode == Walk {
		l.Line, l.Direction = "", ""
		if rl.RunningTime > 0 {
			l.Minutes = int((time.Duration(rl.RunningTime)*time.Second + 30*time.Second) / time.Minute)
		} else {
			l.Minutes = int(l.Arrive.Sub(l.Depart) / time.Minute)
		}
	} else {
		d, x := parseDelay(rl.DepDelay)
		l.DepDelay = d
		l.Cancelled = l.Cancelled || x
		l.Minutes = int(l.Arrive.Sub(l.Depart) / time.Minute)
		t := string(rl.Track)
		l.TrackChanged = strings.Contains(t, "!")
		l.Track = strings.TrimSpace(strings.ReplaceAll(t, "!", ""))
	}
	return l, nil
}

// applyWalkOverrides replaces the first/last walking time when the stop has a configured value.
func applyWalkOverrides(legs []Leg, ov map[string]config.WalkOverride) {
	if len(ov) == 0 || len(legs) == 0 {
		return
	}
	if f := &legs[0]; f.Mode == Walk {
		if o, ok := ov[f.ToStopID]; ok && o.ToStop > 0 {
			f.Minutes = o.ToStop
			f.Depart = f.Arrive.Add(-time.Duration(o.ToStop) * time.Minute)
		}
	}
	if l := &legs[len(legs)-1]; l.Mode == Walk && len(legs) > 1 {
		if o, ok := ov[l.FromStopID]; ok && o.FromStop > 0 {
			l.Minutes = o.FromStop
			l.Arrive = l.Depart.Add(time.Duration(o.FromStop) * time.Minute)
		}
	}
}

// transferRisk checks each change with realtime delays applied.
func transferRisk(legs []Leg) string {
	prev := -1
	walk := 0
	for i, l := range legs {
		if l.Mode == Walk {
			if prev >= 0 {
				walk += l.Minutes
			}
			continue
		}
		if prev >= 0 {
			p := legs[prev]
			ready := p.Arrive.Add(time.Duration(deref(p.ArrDelay)+walk) * time.Minute)
			leaves := l.Depart.Add(time.Duration(deref(l.DepDelay)) * time.Minute)
			if ready.After(leaves) {
				return fmt.Sprintf("Change to %s at %s may not work out", lineLabel(l), l.From)
			}
		}
		prev, walk = i, 0
	}
	return ""
}

func cancelledNote(legs []Leg) string {
	for _, l := range legs {
		if l.Cancelled {
			return fmt.Sprintf("%s from %s is cancelled", lineLabel(l), l.From)
		}
	}
	return "Cancelled"
}

func lineLabel(l Leg) string {
	switch l.Mode {
	case Bus:
		return "Bus " + l.Line
	case Tram:
		return "Tram " + l.Line
	default:
		if l.Line != "" {
			return l.Line
		}
		return string(l.Mode)
	}
}

func collectDisruptions(raw json.RawMessage, into map[string]bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return // empty list or missing
	}
	var m map[string]struct {
		Texts map[string]struct {
			Summary string `json:"summary"`
		} `json:"texts"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	for _, d := range m {
		for _, k := range []string{"S", "M", "L"} {
			if t, ok := d.Texts[k]; ok && t.Summary != "" {
				into[t.Summary] = true
				break
			}
		}
	}
}

func mapMode(t string) Mode {
	switch {
	case t == "walk":
		return Walk
	case t == "bus" || t == "post" || t == "night_bus":
		return Bus
	case t == "tram":
		return Tram
	case t == "ship":
		return Ship
	case t == "cablecar" || t == "gondola" || t == "chairlift" || t == "funicular":
		return Cable
	case strings.Contains(t, "train"):
		return Train
	default:
		return Other
	}
}

// parseDelay handles "+3", "-1", "+0", "X" (cancelled) and missing values.
func parseDelay(s *string) (*int, bool) {
	if s == nil {
		return nil, false
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil, false
	}
	if strings.EqualFold(v, "X") {
		return nil, true
	}
	n, err := strconv.Atoi(strings.TrimPrefix(v, "+"))
	if err != nil {
		return nil, false
	}
	return &n, false
}

func isX(s *string) bool { _, x := parseDelay(s); return x }

func parseTime(s string, loc *time.Location) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05", s, loc)
}

// cleanName drops search.ch's " @x,y" coordinate suffix on addresses.
func cleanName(s string) string {
	if i := strings.Index(s, " @"); i > 0 {
		return s[:i]
	}
	return s
}

func firstVehicle(legs []Leg) int {
	for i, l := range legs {
		if l.Mode != Walk {
			return i
		}
	}
	return -1
}

func lastVehicle(legs []Leg) int {
	for i := len(legs) - 1; i >= 0; i-- {
		if legs[i].Mode != Walk {
			return i
		}
	}
	return -1
}

func countVehicles(legs []Leg) int {
	n := 0
	for _, l := range legs {
		if l.Mode != Walk {
			n++
		}
	}
	return n
}

func connectionID(c Connection) string {
	h := sha1.New()
	for _, l := range c.Legs {
		if l.Mode != Walk {
			fmt.Fprintf(h, "%s|%s|%s;", l.Line, l.FromStopID, l.Depart.Format(time.RFC3339))
		}
	}
	fmt.Fprintf(h, "%s|%s", c.DoorDepart.Format(time.RFC3339), c.DoorArrive.Format(time.RFC3339))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
