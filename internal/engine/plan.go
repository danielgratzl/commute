// Package engine decides which trip the live view shows and ranks connections.
package engine

import (
	"sort"
	"time"

	"commute/internal/config"
	"commute/internal/transit"
)

type Direction string

const (
	ToWork Direction = "h2w"
	ToHome Direction = "w2h"
)

type Mode string

// Live is the only mode: departures from now, with realtime delays.
const Live Mode = "live"

// Plan is what to query and how to label it.
type Plan struct {
	Direction  Direction `json:"direction"`
	Mode       Mode      `json:"mode"`
	Label      string    `json:"label"`
	Reason     string    `json:"reason"` // why this direction is shown
	Auto       bool      `json:"auto"`
	CommuteDay bool      `json:"commute_day"` // today is a usual commute day
}

// Override comes from the UI: an empty direction means "auto".
type Override struct {
	Direction Direction
}

// Decide picks the direction: to work before schedule.switch_to_home, home from then on.
// Departures are always live, from now.
func Decide(now time.Time, cfg *config.Config, ov Override) Plan {
	now = now.In(cfg.Loc)
	sw := cfg.Schedule.SwitchToHome
	p := Plan{Direction: ToWork, Mode: Live, Label: "Now", Auto: true, Reason: "Trip to work until " + sw}
	if now.Hour()*60+now.Minute() >= config.MustClock(sw) {
		p.Direction, p.Reason = ToHome, "Trip home from "+sw
	}
	if ov.Direction != "" && ov.Direction != p.Direction {
		p.Direction, p.Auto, p.Reason = ov.Direction, false, "Chosen manually"
	}
	p.CommuteDay = cfg.IsCommuteDay(now)
	return p
}

// Pick chooses the hero connection and filters the list.
//
// Live: drop connections whose leave time is more than 2 min past; hero is the earliest door
// arrival among non-cancelled connections still catchable (ties: leave later, fewer changes).
// At-risk connections count 5 min later so a safe one arriving nearly as early wins.
func Pick(now time.Time, cs []transit.Connection) (hero string, list []transit.Connection) {
	for _, c := range cs {
		if c.LeaveAt.Before(now.Add(-2 * time.Minute)) {
			continue
		}
		list = append(list, c)
	}
	var cands []transit.Connection
	for _, c := range list {
		if c.Status == transit.StatusCancelled {
			continue
		}
		if c.LeaveAt.Before(now.Add(-30 * time.Second)) {
			continue
		}
		cands = append(cands, c)
	}
	if len(cands) == 0 {
		return "", list
	}
	eff := func(c transit.Connection) time.Time {
		if c.Status == transit.StatusAtRisk {
			return c.DoorArrive.Add(5 * time.Minute)
		}
		return c.DoorArrive
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if !eff(a).Equal(eff(b)) {
			return eff(a).Before(eff(b))
		}
		if !a.LeaveAt.Equal(b.LeaveAt) {
			return a.LeaveAt.After(b.LeaveAt)
		}
		return a.Transfers < b.Transfers
	})
	return cands[0].ID, list
}
