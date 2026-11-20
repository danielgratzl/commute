// Package config loads the YAML configuration and fills in defaults.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Place struct {
	Label string  `yaml:"label" json:"label"`
	Lat   float64 `yaml:"lat" json:"lat"`
	Lon   float64 `yaml:"lon" json:"lon"`
}

// WalkOverride replaces the router's walking time between a stop and the door.
// ToStop is door -> stop, FromStop is stop -> door (they differ on hills).
type WalkOverride struct {
	ToStop   int `yaml:"to_stop"`
	FromStop int `yaml:"from_stop"`
}

type Schedule struct {
	ArriveWork         string   `yaml:"arrive_work"`           // usual arrival at work, e.g. "09:00"
	LeaveWork          string   `yaml:"leave_work"`            // usual departure from work, e.g. "16:00"
	SwitchHomeAfter    int      `yaml:"switch_home_after_min"` // show trip home from arrive_work + N min
	EveningSwitch      string   `yaml:"evening_switch"`        // after this, show tomorrow's trip to work
	LiveBeforeLeaveMin int      `yaml:"live_before_leave_min"` // before leave_work - N min, show planned trips around leave_work
	CommuteDays        []string `yaml:"commute_days"`          // label only; the app works every day
}

type Transit struct {
	Provider          string                  `yaml:"provider"`
	Num               int                     `yaml:"num"`
	DailyRouteBudget  int                     `yaml:"daily_route_budget"`
	LiveTTLSeconds    int                     `yaml:"live_ttl_seconds"`
	PlannedTTLSeconds int                     `yaml:"planned_ttl_seconds"`
	WalkOverrides     map[string]WalkOverride `yaml:"walk_overrides"` // keyed by stop id
}

// Bike describes the rider. Ride time comes from a physics model calibrated to FlatKmh.
type Bike struct {
	Enabled        bool    `yaml:"enabled"`
	Label          string  `yaml:"label"`
	Profile        string  `yaml:"profile"`          // BRouter profile, e.g. fastbike
	FlatKmh        float64 `yaml:"flat_speed_kmh"`   // cruising speed on the flat
	MaxKmh         float64 `yaml:"max_speed_kmh"`    // descents
	MassKg         float64 `yaml:"mass_kg"`          // rider + bike
	HandlingOutMin int     `yaml:"handling_out_min"` // get the bike out, helmet on
	HandlingInMin  int     `yaml:"handling_in_min"`  // lock up, change
	SignalPenaltyS int     `yaml:"signal_penalty_s"` // average wait per traffic light
	KCal           float64 `yaml:"k_cal"`            // personal correction, 1.1 = 10% slower
}

type Config struct {
	Timezone  string `yaml:"timezone"`
	Listen    string `yaml:"listen"`
	DataDir   string `yaml:"data_dir"`
	BufferMin int    `yaml:"buffer_min"`
	Places    struct {
		Home Place `yaml:"home"`
		Work Place `yaml:"work"`
	} `yaml:"places"`
	Schedule Schedule `yaml:"schedule"`
	Transit  Transit  `yaml:"transit"`
	Bike     Bike     `yaml:"bike"`

	Loc *time.Location `yaml:"-"`
}

func Default() Config {
	var c Config
	c.Timezone = "Europe/Zurich"
	c.Listen = ":8080"
	c.DataDir = "/data"
	c.BufferMin = 2
	c.Places.Home = Place{Label: "Home", Lat: 47.3780, Lon: 8.5402}
	c.Places.Work = Place{Label: "Work", Lat: 47.4083, Lon: 8.5067}
	c.Schedule = Schedule{
		ArriveWork: "09:00", LeaveWork: "16:00", SwitchHomeAfter: 60,
		EveningSwitch: "20:00", LiveBeforeLeaveMin: 45,
		CommuteDays: []string{"tue", "thu"},
	}
	c.Bike = Bike{Enabled: true, Label: "Road bike", Profile: "fastbike", FlatKmh: 30, MaxKmh: 45, MassKg: 85,
		HandlingOutMin: 3, HandlingInMin: 3, SignalPenaltyS: 10, KCal: 1.0}
	c.Transit = Transit{Provider: "searchch", Num: 6, DailyRouteBudget: 800, LiveTTLSeconds: 120, PlannedTTLSeconds: 900}
	return c
}

// Load reads path (if it exists) over the defaults. Env vars LISTEN and DATA_DIR win over the file.
func Load(path string) (Config, error) {
	c := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(b, &c); err != nil {
				return c, fmt.Errorf("parse %s: %w", path, err)
			}
		case os.IsNotExist(err):
			// run on defaults
		default:
			return c, err
		}
	}
	if v := os.Getenv("LISTEN"); v != "" {
		c.Listen = v
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		c.DataDir = v
	}
	return c, c.validate()
}

func (c *Config) validate() error {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return fmt.Errorf("timezone %q: %w", c.Timezone, err)
	}
	c.Loc = loc
	for name, v := range map[string]string{"arrive_work": c.Schedule.ArriveWork, "leave_work": c.Schedule.LeaveWork, "evening_switch": c.Schedule.EveningSwitch} {
		if _, err := ParseClock(v); err != nil {
			return fmt.Errorf("schedule.%s: %w", name, err)
		}
	}
	if c.Bike.FlatKmh <= 5 || c.Bike.FlatKmh > 60 {
		return fmt.Errorf("bike.flat_speed_kmh: want 5..60, got %v", c.Bike.FlatKmh)
	}
	if c.Bike.KCal <= 0 {
		c.Bike.KCal = 1
	}
	if c.Bike.MaxKmh < c.Bike.FlatKmh {
		c.Bike.MaxKmh = c.Bike.FlatKmh
	}
	if c.Transit.Num <= 0 {
		c.Transit.Num = 6
	}
	return nil
}

// ParseClock turns "HH:MM" into minutes after midnight.
func ParseClock(s string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("want HH:MM, got %q", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// MustClock is ParseClock for values already validated.
func MustClock(s string) int {
	m, _ := ParseClock(s)
	return m
}

// IsCommuteDay reports whether t's weekday is listed in commute_days.
func (c *Config) IsCommuteDay(t time.Time) bool {
	wd := strings.ToLower(t.Weekday().String()[:3])
	for _, d := range c.Schedule.CommuteDays {
		d = strings.ToLower(strings.TrimSpace(d))
		if len(d) >= 3 && d[:3] == wd {
			return true
		}
	}
	return false
}
