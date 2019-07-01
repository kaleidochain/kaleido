// Copyright (c) 2019 The kaleido Authors
// This file is part of kaleido
//
// kaleido is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// kaleido is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with kaleido. If not, see <https://www.gnu.org/licenses/>.

package core

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
)

const (
	tickTockBufferSize = 10
)

// internally generated messages which may update the state
type TimeoutInfo struct {
	Duration uint64 `json:"duration"`
	Height   uint64 `json:"height"`
	Round    uint32 `json:"round"`
	Step     uint32 `json:"step"`
}

func (ti *TimeoutInfo) String() string {
	return fmt.Sprintf("%v ; %d/%d/%d", ti.Duration, ti.Height, ti.Round, ti.Step)
}

// TimeoutTicker is a timer that schedules timeouts
// conditional on the height/round/step in the TimeoutInfo.
// The TimeoutInfo.Duration may be non-positive.
type TimeoutTicker interface {
	Start() error
	Stop()
	Chan() <-chan TimeoutInfo       // on which to receive a timeout
	ScheduleTimeout(ti TimeoutInfo) // reset the timer
}

// timeoutTicker wraps time.Timer,
// scheduling timeouts only for greater height/round/step
// than what it's already seen.
// Timeouts are scheduled along the tickChan,
// and fired on the tockChan.
type timeoutTicker struct {
	quit     chan struct{}
	timer    *time.Timer
	tickChan chan TimeoutInfo // for scheduling timeouts
	tockChan chan TimeoutInfo // for notifying about them
}

// NewTimeoutTicker returns a new TimeoutTicker.
func NewTimeoutTicker() TimeoutTicker {
	tt := &timeoutTicker{
		quit:     make(chan struct{}),
		timer:    time.NewTimer(0),
		tickChan: make(chan TimeoutInfo, tickTockBufferSize),
		tockChan: make(chan TimeoutInfo, tickTockBufferSize),
	}
	tt.stopTimer() // don't want to fire until the first scheduled timeout
	return tt
}

// OnStart implements cmn.Service. It starts the timeout routine.
func (t *timeoutTicker) Start() error {
	go t.timeoutRoutine()

	return nil
}

// OnStop implements cmn.Service. It stops the timeout routine.
func (t *timeoutTicker) Stop() {
	close(t.quit)
	t.stopTimer()
}

// Chan returns a channel on which timeouts are sent.
func (t *timeoutTicker) Chan() <-chan TimeoutInfo {
	return t.tockChan
}

// ScheduleTimeout schedules a new timeout by sending on the internal tickChan.
// The timeoutRoutine is always available to read from tickChan, so this won't block.
// The scheduling may fail if the timeoutRoutine has already scheduled a timeout for a later height/round/step.
func (t *timeoutTicker) ScheduleTimeout(ti TimeoutInfo) {
	t.tickChan <- ti
}

//-------------------------------------------------------------

// stop the timer and drain if necessary
func (t *timeoutTicker) stopTimer() {
	// Stop() returns false if it was already fired or was stopped
	if !t.timer.Stop() {
		select {
		case <-t.timer.C:
		default:
			log.Debug("Timer already stopped")
		}
	}
}

// send on tickChan to start a new timer.
// timers are interupted and replaced by new ticks from later steps
// timeouts of 0 on the tickChan will be immediately relayed to the tockChan
func (t *timeoutTicker) timeoutRoutine() {
	log.Debug("Starting timeout routine")
	var ti TimeoutInfo
	for {
		select {
		case newti := <-t.tickChan:
			log.Debug("Received tick", "old_ti", ti, "new_ti", newti)

			// ignore tickers for old height/round/step
			if newti.Height < ti.Height {
				continue
			} else if newti.Height == ti.Height {
				if newti.Round < ti.Round {
					continue
				} else if newti.Round == ti.Round {
					if ti.Step > 0 && newti.Step <= ti.Step {
						continue
					}
				}
			}

			// stop the last timer
			t.stopTimer()

			// update TimeoutInfo and reset timer
			// NOTE time.Timer allows duration to be non-positive
			ti = newti
			t.timer.Reset(time.Duration(ti.Duration))
			log.Debug("Scheduled timeout", "dur", ti.Duration, "height", ti.Height, "round", ti.Round, "step", ti.Step)
		case <-t.timer.C:
			log.Debug("Timed out", "dur", ti.Duration, "height", ti.Height, "round", ti.Round, "step", ti.Step)
			// go routine here guarantees timeoutRoutine doesn't block.
			// Determinism comes from playback in the receiveRoutine.
			// We can eliminate it by merging the timeoutRoutine into receiveRoutine
			//  and managing the timeouts ourselves with a millisecond ticker
			go func(toi TimeoutInfo) { t.tockChan <- toi }(ti)
		case <-t.quit:
			return
		}
	}
}
