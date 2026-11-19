package engine

import (
	"os"
	"testing"
	"time"

	"commute/internal/config"
	"commute/internal/transit"
)

func cfg(t *testing.T) *config.Config {
	t.Helper()
	c, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	return &c
}

func at(t *testing.T, c *config.Config, s string) time.Time {
	t.Helper()
	v, err := time.ParseInLocation("2006-01-02 15:04", s, c.Loc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDecide(t *testing.T) {
	c := cfg(t) // arrive 09:00, leave 16:00, home after +60, live from 15:15, evening 20:00
	cases := []struct {
		now   string
		dir   Direction
		mode  Mode
		at    string
		label string
	}{
		{"2026-11-19 00:30", ToWork, Planned, "2026-11-19 07:45", "Today, arrive by 09:00"},
		{"2026-11-19 06:00", ToWork, Planned, "2026-11-19 07:45", "Today, arrive by 09:00"},
		{"2026-11-19 07:14", ToWork, Planned, "2026-11-19 07:45", "Today, arrive by 09:00"},
		{"2026-11-19 07:15", ToWork, Live, "", "Now"},
		{"2026-11-19 07:30", ToWork, Live, "", "Now"},
		{"2026-11-19 09:59", ToWork, Live, "", "Now"},
		{"2026-11-19 10:00", ToHome, Planned, "2026-11-19 15:45", "Today, around 16:00"},
		{"2026-11-19 15:14", ToHome, Planned, "2026-11-19 15:45", "Today, around 16:00"},
		{"2026-11-19 15:15", ToHome, Live, "", "Now"},
		{"2026-11-19 19:30", ToHome, Live, "", "Now"},
		{"2026-11-19 20:00", ToWork, Planned, "2026-11-20 07:45", "Tomorrow, Fri 20 Nov, arrive by 09:00"},
		{"2026-11-19 23:50", ToWork, Planned, "2026-11-20 07:45", "Tomorrow, Fri 20 Nov, arrive by 09:00"},
	}
	for _, tc := range cases {
		p := Decide(at(t, c, tc.now), c, Override{})
		if p.Direction != tc.dir || p.Mode != tc.mode || p.Label != tc.label {
			t.Errorf("%s: got %s/%s %q, want %s/%s %q", tc.now, p.Direction, p.Mode, p.Label, tc.dir, tc.mode, tc.label)
		}
		if tc.at != "" && !p.At.Equal(at(t, c, tc.at)) {
			t.Errorf("%s: at %v, want %s", tc.now, p.At, tc.at)
		}
	}
}

func TestCommuteDayLabel(t *testing.T) {
	c := cfg(t)
	// Wed evening -> shows Thu morning, a commute day
	if p := Decide(at(t, c, "2026-11-18 21:00"), c, Override{}); !p.CommuteDay {
		t.Error("Thu should be a commute day")
	}
	// Thu evening -> Fri morning, not a commute day, but still shown
	p := Decide(at(t, c, "2026-11-19 21:00"), c, Override{})
	if p.CommuteDay || p.Direction != ToWork {
		t.Errorf("Fri: %+v", p)
	}
}

func TestOverride(t *testing.T) {
	c := cfg(t)
	p := Decide(at(t, c, "2026-11-19 11:00"), c, Override{Direction: ToWork})
	if p.Direction != ToWork || p.Mode != Live || p.Auto {
		t.Errorf("manual direction: %+v", p)
	}
	p = Decide(at(t, c, "2026-11-19 11:00"), c, Override{Now: true})
	if p.Direction != ToHome || p.Mode != Live || p.Auto {
		t.Errorf("force now: %+v", p)
	}
}

func load(t *testing.T, c *config.Config, name string) []transit.Connection {
	b, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := transit.Parse(b, transit.NormaliseOpts{Loc: c.Loc, BufferMin: 2})
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func TestPickSkipsCancelled(t *testing.T) {
	c := cfg(t)
	cs := load(t, c, "searchch_h2w_cancelled_tram.json")
	now := at(t, c, "2026-11-20 19:40")
	hero, list := Pick(now, Plan{Mode: Live}, cs)
	var h transit.Connection
	for _, x := range list {
		if x.ID == hero {
			h = x
		}
	}
	if h.ID == "" {
		t.Fatal("no hero")
	}
	if h.Status == transit.StatusCancelled {
		t.Fatal("hero is cancelled")
	}
}

func TestPickArriveBy(t *testing.T) {
	c := cfg(t)
	cs := load(t, c, "searchch_h2w_tomorrow_arriveby.json")
	p := Plan{Mode: Planned, ArriveBy: at(t, c, "2026-11-20 09:00")}
	hero, list := Pick(at(t, c, "2026-11-19 21:00"), p, cs)
	var h transit.Connection
	onTime := false
	earliest := list[0].DoorArrive
	for _, x := range list {
		if x.ID == hero {
			h = x
		}
		if !x.DoorArrive.After(p.ArriveBy) {
			onTime = true
		}
		if x.DoorArrive.Before(earliest) {
			earliest = x.DoorArrive
		}
	}
	if h.ID == "" {
		t.Fatal("no hero")
	}
	if onTime && h.DoorArrive.After(p.ArriveBy) {
		t.Errorf("hero arrives %s although an on-time connection exists", h.DoorArrive.Format("15:04"))
	}
	if !onTime && !h.DoorArrive.Equal(earliest) {
		t.Errorf("nothing on time; hero should be the earliest arrival, got %s", h.DoorArrive.Format("15:04"))
	}

	// With a later target the hero is the latest connection that still makes it.
	p.ArriveBy = at(t, c, "2026-11-20 09:30")
	hero, list = Pick(at(t, c, "2026-11-19 21:00"), p, cs)
	for _, x := range list {
		if x.ID == hero && x.DoorDepart.Format("15:04") != "08:31" {
			t.Errorf("arrive by 09:30: hero leaves %s, want 08:31 (arr 09:19)", x.DoorDepart.Format("15:04"))
		}
	}
}
