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
	c := cfg(t) // switch_to_home 09:00
	cases := []struct {
		now string
		dir Direction
	}{
		{"2026-11-19 00:30", ToWork},
		{"2026-11-19 06:30", ToWork},
		{"2026-11-19 08:59", ToWork},
		{"2026-11-19 09:00", ToHome},
		{"2026-11-19 12:00", ToHome},
		{"2026-11-19 23:50", ToHome},
	}
	for _, tc := range cases {
		p := Decide(at(t, c, tc.now), c, Override{})
		if p.Direction != tc.dir || p.Mode != Live || p.Label != "Now" || !p.Auto {
			t.Errorf("%s: got %+v, want %s live", tc.now, p, tc.dir)
		}
	}
}

func TestCommuteDayLabel(t *testing.T) {
	c := cfg(t)
	if p := Decide(at(t, c, "2026-11-19 07:00"), c, Override{}); !p.CommuteDay {
		t.Error("Thu should be a commute day")
	}
	if p := Decide(at(t, c, "2026-11-20 07:00"), c, Override{}); p.CommuteDay || p.Direction != ToWork {
		t.Errorf("Fri: %+v", p)
	}
}

func TestOverride(t *testing.T) {
	c := cfg(t)
	p := Decide(at(t, c, "2026-11-19 11:00"), c, Override{Direction: ToWork})
	if p.Direction != ToWork || p.Mode != Live || p.Auto {
		t.Errorf("manual direction: %+v", p)
	}
	// Choosing the direction auto would pick anyway stays auto.
	if p := Decide(at(t, c, "2026-11-19 11:00"), c, Override{Direction: ToHome}); !p.Auto {
		t.Errorf("same as auto should stay auto: %+v", p)
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
	hero, list := Pick(now, cs)
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
