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

// TripWeather is filled in by the weather phase; nil until then.
type TripWeather struct{}

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
	At               time.Time // zero = now
	Num              int
}
