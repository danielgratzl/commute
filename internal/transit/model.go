// Package transit fetches public transport connections and normalises them
// into door-to-door trips.
package transit

import "time"

type Mode string

const (
	Walk  Mode = "walk"
	Bus   Mode = "bus"
	Tram  Mode = "tram"
	Train Mode = "train"
	Ship  Mode = "ship"
	Cable Mode = "cable"
	Other Mode = "other"
)

type Status string

const (
	StatusOK        Status = "ok"
	StatusAtRisk    Status = "at_risk"   // a transfer may not work out
	StatusCancelled Status = "cancelled" // a leg is cancelled; never recommend
)

type Leg struct {
	Mode         Mode      `json:"mode"`
	Line         string    `json:"line,omitempty"`
	Direction    string    `json:"direction,omitempty"` // terminal of the vehicle
	From         string    `json:"from"`
	To           string    `json:"to"`
	FromStopID   string    `json:"from_stop_id,omitempty"`
	ToStopID     string    `json:"to_stop_id,omitempty"`
	Depart       time.Time `json:"depart"`              // planned (walk legs: actual start incl. overrides)
	Arrive       time.Time `json:"arrive"`              // planned
	DepDelay     *int      `json:"dep_delay,omitempty"` // minutes; nil = no realtime data
	ArrDelay     *int      `json:"arr_delay,omitempty"`
	Cancelled    bool      `json:"cancelled,omitempty"`
	Track        string    `json:"track,omitempty"`
	TrackChanged bool      `json:"track_changed,omitempty"`
	Minutes      int       `json:"minutes"`
	FgColor      string    `json:"fg,omitempty"`
	BgColor      string    `json:"bg,omitempty"`
	FromLat      float64   `json:"from_lat,omitempty"`
	FromLon      float64   `json:"from_lon,omitempty"`
	ToLat        float64   `json:"to_lat,omitempty"`
	ToLon        float64   `json:"to_lon,omitempty"`
}

// PointWeather is the weather at one end of the trip at the time you are there.
type PointWeather struct {
	Place   string    `json:"place"` // "origin" | "destination"
	At      time.Time `json:"at"`
	Temp    float64   `json:"temp"`
	Feels   float64   `json:"feels"`
	RateMMH float64   `json:"rate_mm_h"` // deterministic model, hourly resolution
	Chance  int       `json:"chance"`    // % chance of rain in that hour
	Snow    bool      `json:"snow,omitempty"`
}

// OutdoorInterval is a stretch of the trip spent outside: a walk or a wait at a stop.
type OutdoorInterval struct {
	Kind    string    `json:"kind"` // "walk" | "wait"
	Leg     int       `json:"leg"`  // walk: index of the leg; wait: index of the leg before the wait
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Place   string    `json:"place"`     // "origin" | "destination" (nearest forecast point)
	Chance  int       `json:"chance"`    // % of ensemble members with rain during this stretch
	RateMMH float64   `json:"rate_mm_h"` // typical intensity if it rains
}

// TripWeather summarises rain and temperature for the minutes you are outside.
type TripWeather struct {
	Origin     PointWeather      `json:"origin"`
	Dest       PointWeather      `json:"destination"`
	OutdoorMin int               `json:"outdoor_min"`
	Chance     int               `json:"chance"`    // % chance of rain while you are outside
	RateMMH    float64           `json:"rate_mm_h"` // typical intensity if it rains
	Intensity  string            `json:"intensity"` // none | light | moderate | heavy
	Verdict    string            `json:"verdict"`   // dry | maybe | wet
	Headline   string            `json:"headline"`
	Detail     string            `json:"detail"`
	Snow       bool              `json:"snow,omitempty"`
	Ice        bool              `json:"ice,omitempty"`
	GustKmh    float64           `json:"gust_kmh,omitempty"` // peak gust while outside
	Dark       bool              `json:"dark,omitempty"`     // part of the time outside is before sunrise or after sunset
	Intervals  []OutdoorInterval `json:"intervals"`
	Source     string            `json:"source"`
}

type Connection struct {
	ID            string       `json:"id"`
	LeaveAt       time.Time    `json:"leave_at"`    // door departure minus buffer
	DoorDepart    time.Time    `json:"door_depart"` // step out of the door
	DoorArrive    time.Time    `json:"door_arrive"` // incl. realtime delays
	PlannedArrive time.Time    `json:"planned_arrive"`
	Minutes       int          `json:"minutes"`   // door to door
	DelayMin      int          `json:"delay_min"` // arrival delay vs plan
	Transfers     int          `json:"transfers"`
	Status        Status       `json:"status"`
	StatusNote    string       `json:"status_note,omitempty"`
	Realtime      bool         `json:"realtime_alternative,omitempty"` // search.ch re-planned this because of a disruption
	HasRealtime   bool         `json:"has_realtime"`                   // any leg carries live delay data
	Disruptions   []string     `json:"disruptions,omitempty"`
	Legs          []Leg        `json:"legs"`
	Weather       *TripWeather `json:"weather"`
}

// Query describes one routing request.
type Query struct {
	FromLat, FromLon float64
	ToLat, ToLon     float64
	At               time.Time // zero = now (the app always queries now)
	Num              int
}
