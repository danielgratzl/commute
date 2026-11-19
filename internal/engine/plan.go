// Package engine decides which trip the live view shows and ranks connections.
package engine

import (
	"fmt"
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

const (
	Live    Mode = "live"    // departures from now, with realtime delays
	Planned Mode = "planned" // around a usual time, possibly tomorrow
)

// Plan is what to query and how to label it.
type Plan struct {
	Direction  Direction `json:"direction"`
	Mode       Mode      `json:"mode"`
	At         time.Time `json:"at,omitzero"`        // planned query start (zero for live)
	ArriveBy   time.Time `json:"arrive_by,omitzero"` // target for picking the hero in planned-to-work mode
	Label      string    `json:"label"`              // e.g. "Tomorrow, Fri 20 Nov"
	Reason     string    `json:"reason"`             // why this view is shown
	Auto       bool      `json:"auto"`
	CommuteDay bool      `json:"commute_day"` // the day of the shown trip is a usual commute day
}

// Override comes from the UI: empty fields mean "auto".
type Override struct {
	Direction Direction
	Now       bool // force live departures
}

// Decide picks direction and mode for the given time.
//
//	before arrive_work - 60 - live_before   -> to work today, planned to arrive by arrive_work
//	before arrive_work + switch_home_after   -> to work, live
//	until leave_work - live_before_leave_min -> home, planned around leave_work
//	until evening_switch                     -> home, live
//	after evening_switch                     -> to work tomorrow, planned for arrive_work
func Decide(now time.Time, cfg *config.Config, ov Override) Plan {
	now = now.In(cfg.Loc)
	s := cfg.Schedule
	min := now.Hour()*60 + now.Minute()
	arrive := config.MustClock(s.ArriveWork)
	leave := config.MustClock(s.LeaveWork)
	homeFrom := arrive + s.SwitchHomeAfter
	evening := config.MustClock(s.EveningSwitch)
	plannedHomeUntil := leave - s.LiveBeforeLeaveMin
	plannedWorkUntil := arrive - 60 - s.LiveBeforeLeaveMin

	var p Plan
	switch {
	case min < plannedWorkUntil:
		p = Plan{Direction: ToWork, Mode: Planned, At: atClock(now, arrive-75), ArriveBy: atClock(now, arrive),
			Reason: "Arrive by " + s.ArriveWork}
	case min < homeFrom:
		p = Plan{Direction: ToWork, Mode: Live, Reason: "Trip to work until " + clock(homeFrom)}
	case min < plannedHomeUntil:
		p = Plan{Direction: ToHome, Mode: Planned, At: atClock(now, leave-15), Reason: "Around your usual departure at " + s.LeaveWork}
	case min < evening:
		p = Plan{Direction: ToHome, Mode: Live, Reason: "Trip home until " + s.EveningSwitch}
	default:
		tomorrow := atClock(now, 0).AddDate(0, 0, 1)
		p = Plan{Direction: ToWork, Mode: Planned, At: atClock(tomorrow, arrive-75), ArriveBy: atClock(tomorrow, arrive),
			Reason: "Arrive by " + s.ArriveWork + " tomorrow"}
	}
	p.Auto = true

	if ov.Direction != "" && ov.Direction != p.Direction {
		p.Auto = false
		p.Direction = ov.Direction
		p.Mode, p.At, p.ArriveBy = Live, time.Time{}, time.Time{}
		p.Reason = "Chosen manually"
	}
	if ov.Now && p.Mode != Live {
		p.Auto = false
		p.Mode, p.At, p.ArriveBy = Live, time.Time{}, time.Time{}
		p.Reason = "Departures from now"
	}

	day := now
	if !p.At.IsZero() {
		day = p.At
	}
	p.CommuteDay = cfg.IsCommuteDay(day)
	p.Label = label(now, p)
	return p
}

func label(now time.Time, p Plan) string {
	if p.Mode == Live {
		return "Now"
	}
	day := "Today"
	if !sameDay(now, p.At) {
		day = "Tomorrow, " + p.At.Format("Mon 2 Jan")
	}
	if !p.ArriveBy.IsZero() {
		return day + ", arrive by " + p.ArriveBy.Format("15:04")
	}
	return day + ", around " + p.At.Add(15*time.Minute).Format("15:04")
}

// Pick chooses the hero connection and filters the list.
//
// Live: drop connections whose leave time is more than 2 min past; hero is the earliest door
// arrival among non-cancelled connections still catchable (ties: leave later, fewer changes).
// At-risk connections count 5 min later so a safe one arriving nearly as early wins.
// Planned to work: hero is the latest connection arriving by ArriveBy.
func Pick(now time.Time, p Plan, cs []transit.Connection) (hero string, list []transit.Connection) {
	for _, c := range cs {
		if p.Mode == Live && c.LeaveAt.Before(now.Add(-2*time.Minute)) {
			continue
		}
		list = append(list, c)
	}
	var cands []transit.Connection
	for _, c := range list {
		if c.Status == transit.StatusCancelled {
			continue
		}
		if p.Mode == Live && c.LeaveAt.Before(now.Add(-30*time.Second)) {
			continue
		}
		cands = append(cands, c)
	}
	if len(cands) == 0 {
		return "", list
	}
	if !p.ArriveBy.IsZero() {
		var best *transit.Connection
		for i := range cands {
			c := &cands[i]
			if !c.DoorArrive.After(p.ArriveBy) && (best == nil || c.LeaveAt.After(best.LeaveAt)) {
				best = c
			}
		}
		if best != nil {
			return best.ID, list
		}
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

func atClock(day time.Time, minutes int) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, minutes/60, minutes%60, 0, 0, day.Location())
}

func clock(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
