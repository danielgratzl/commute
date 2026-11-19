package transit

import (
	"os"
	"testing"
	"time"

	"commute/internal/config"
)

// Fixtures are recorded search.ch responses for a neutral route between two public
// places: "Home" = Kunstmuseum Winterthur, "Work" = ETH Hönggerberg.

func zurich(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func parseFixture(t *testing.T, name string, o NormaliseOpts) []Connection {
	t.Helper()
	b, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := Parse(b, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) == 0 {
		t.Fatal("no connections parsed")
	}
	return cs
}

func TestCancelledTramIsFlagged(t *testing.T) {
	cs := parseFixture(t, "searchch_h2w_cancelled_tram.json", NormaliseOpts{Loc: zurich(t), BufferMin: 2})
	c := cs[0]
	if c.Status != StatusCancelled {
		t.Fatalf("first connection: status %q, want cancelled (%s)", c.Status, c.StatusNote)
	}
	if c.StatusNote != "Tram 7 from Stettbach, Bahnhof is cancelled" {
		t.Errorf("note = %q", c.StatusNote)
	}
	// The trailing address marker must not become a leg.
	last := c.Legs[len(c.Legs)-1]
	if last.Mode != Walk || last.To != "Work address" {
		t.Errorf("last leg = %+v", last)
	}
	if c.Legs[0].From != "Home address" {
		t.Errorf("address suffix not stripped: %q", c.Legs[0].From)
	}
}

func TestDoorToDoorAndBuffer(t *testing.T) {
	cs := parseFixture(t, "searchch_h2w_cancelled_tram.json", NormaliseOpts{Loc: zurich(t), BufferMin: 2})
	c := cs[0] // 19:59 -> 20:49: walk, S11, walk, tram 7, bus 69, walk
	if got := c.DoorDepart.Format("15:04"); got != "19:59" {
		t.Errorf("door depart %s", got)
	}
	if got := c.LeaveAt.Format("15:04"); got != "19:57" {
		t.Errorf("leave at %s", got)
	}
	if c.Minutes != 50 {
		t.Errorf("minutes %d, want 50", c.Minutes)
	}
	if c.Transfers != 2 {
		t.Errorf("transfers %d, want 2", c.Transfers)
	}
}

func TestWalkOverrideByStopID(t *testing.T) {
	ov := map[string]config.WalkOverride{"8506000": {ToStop: 4, FromStop: 5}} // Winterthur station
	cs := parseFixture(t, "searchch_h2w_cancelled_tram.json", NormaliseOpts{Loc: zurich(t), WalkOverrides: ov})
	c := cs[0]
	if c.Legs[0].Minutes != 4 || c.DoorDepart.Format("15:04") != "20:05" {
		t.Errorf("override not applied: %d min, door %s", c.Legs[0].Minutes, c.DoorDepart.Format("15:04"))
	}
}

func TestDelaysAndAtRisk(t *testing.T) {
	cs := parseFixture(t, "searchch_h2w_now.json", NormaliseOpts{Loc: zurich(t)})
	var found bool
	for _, c := range cs {
		// 20:01 S8 (+1 on arrival) -> bus 80: search.ch marks infeasibility_level 1
		if c.DoorDepart.Format("15:04") == "20:01" && c.Legs[1].Line == "S8" && c.Legs[3].Line == "80" {
			found = true
			if c.Status != StatusAtRisk {
				t.Errorf("S8+80 status %q, want at_risk", c.Status)
			}
			if d := c.Legs[1].ArrDelay; d == nil || *d != 1 {
				t.Errorf("arr delay %v, want +1", d)
			}
			if d := c.Legs[1].DepDelay; d == nil || *d != 0 {
				t.Errorf("dep delay %v, want +0", d)
			}
		}
	}
	if !found {
		t.Fatal("did not find the 20:01 S8 connection")
	}
}

func TestPlannedHasNoRealtime(t *testing.T) {
	cs := parseFixture(t, "searchch_h2w_tomorrow_arriveby.json", NormaliseOpts{Loc: zurich(t)})
	for _, c := range cs {
		if c.HasRealtime {
			t.Errorf("%s: unexpected realtime data in a past timetable search", c.DoorDepart.Format("15:04"))
		}
		if c.Status == StatusCancelled {
			t.Errorf("planned connection marked cancelled")
		}
	}
}

func TestUniqueIDsAndSorted(t *testing.T) {
	cs := parseFixture(t, "searchch_w2h_now.json", NormaliseOpts{Loc: zurich(t)})
	ids := map[string]bool{}
	for i, c := range cs {
		if ids[c.ID] {
			t.Errorf("duplicate id %s", c.ID)
		}
		ids[c.ID] = true
		if i > 0 && c.LeaveAt.Before(cs[i-1].LeaveAt) {
			t.Errorf("not sorted at %d", i)
		}
	}
}

func TestParseDelay(t *testing.T) {
	s := func(v string) *string { return &v }
	cases := []struct {
		in   *string
		want *int
		x    bool
	}{{nil, nil, false}, {s(""), nil, false}, {s("X"), nil, true}, {s("+0"), ip(0), false}, {s("+4"), ip(4), false}, {s("-1"), ip(-1), false}}
	for _, c := range cases {
		got, x := parseDelay(c.in)
		if x != c.x || (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("parseDelay(%v) = %v,%v", c.in, got, x)
		}
	}
}

func ip(i int) *int { return &i }
