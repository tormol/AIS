package logger

import (
	"sync"
	"time"

	"github.com/cenkalti/backoff"
)

const (
	periodicMinSleep = 2 * time.Second
	periodicMaxSleep = 365 * 24 * time.Hour // FIXME max representable
)

// DebugPeriodicIntervals enables logging of periodic-logger intervals.
// After each logger is run the time until next run of that periodic logger is
// printed, as well as the time until any other logger if non-zero.
var DebugPeriodicIntervals = false

type loggerFunc func(l *Composer, sinceLast time.Duration)

// PeriodicLogger is a function that is ran periodically by a logger
type periodicLogger struct {
	id       string
	logger   loggerFunc
	interval backoff.ExponentialBackOff
	nextRun  time.Time
	lastRun  time.Time
}

// groups related fields in Logger
type periodic struct {
	timer   *time.Timer
	loggers []*periodicLogger
	m       sync.Mutex
	stop    bool // tell periodicRunner() to exit
}

func newPeriodic() periodic {
	return periodic{
		timer: time.NewTimer(periodicMaxSleep),
	}
	// NewLogger starts periodicRunner()
}
func (p *periodic) Close() {
	p.m.Lock()
	defer p.m.Unlock()

	p.stop = true
	p.timer.Stop()
	p.timer.Reset(0)
}

// Find the logger with the least time remaining until it should be run,
// and update the timer to fire then.
func resetTimer(l *Logger, now time.Time) {
	next := now.Add(periodicMaxSleep)
	for _, pl := range l.p.loggers {
		if next.After(pl.nextRun) {
			next = pl.nextRun
		}
	}
	if DebugPeriodicIntervals {
		l.Debug("(%s until next periodic logger)", RoundDuration(next.Sub(now), time.Second/1000))
	}
	l.p.timer.Stop() // the channel is immediately drained by periodicRunner().
	l.p.timer.Reset(next.Sub(now))
}

// Run all loggers that want to be run before (now + minSleep)
func runPeriodic(l *Logger, minSleep time.Duration, started time.Time) {
	c := l.Compose(Info)
	defer c.Close()
	limit := started.Add(minSleep)
	for _, pl := range l.p.loggers {
		if limit.After(pl.nextRun) {
			pl.logger(&c, started.Sub(pl.lastRun))
			pl.lastRun = started
			next := pl.interval.NextBackOff()
			if next <= 0 {
				// Cannot use l.Warn() because l.writeLock is locked by c
				l.prefixMessage(Warning)
				c.Writeln("Stopping periodic logger %s", pl.id)
				next = periodicMaxSleep
			}
			if DebugPeriodicIntervals {
				c.Writeln("(%s until next %s)", RoundDuration(next, time.Second), pl.id)
			}
			pl.nextRun = started.Add(next)
		}
	}
}

// Runs until l.p.stop is true
func periodicRunner(l *Logger) {
	for {
		now := <-l.p.timer.C
		// Somebody else could take the lock here, but then no loggers will be run.
		l.p.m.Lock()
		if l.p.stop {
			l.p.m.Unlock()
			break
		}
		runPeriodic(l, periodicMinSleep, now)
		resetTimer(l, now)
		l.p.m.Unlock()
	}
}

// RunAllPeriodic runs all the closures right now, ignoring any intervals.
func (l *Logger) RunAllPeriodic() {
	l.p.m.Lock()
	defer l.p.m.Unlock()
	n := time.Now()
	runPeriodic(l, periodicMaxSleep, n)
	resetTimer(l, n)
}

// AddPeriodic stores a closure that will be called periodically
// with an interval that increases from minInterval to maxInterval exponentally.
func (l *Logger) AddPeriodic(id string, minInterval, maxInterval time.Duration, f loggerFunc) {
	b := backoff.ExponentialBackOff{
		InitialInterval:     minInterval,
		MaxInterval:         maxInterval,
		Multiplier:          3.0,
		RandomizationFactor: 0.0,
		MaxElapsedTime:      0, // disabled
		Clock:               backoff.SystemClock,
	}
	b.Reset()

	l.p.m.Lock()
	defer l.p.m.Unlock()

	for _, p := range l.p.loggers {
		if p.id == id {
			l.Error("A periodic logger with ID %s already exists", id)
			return
		}
	}
	added := time.Now()
	l.p.loggers = append(l.p.loggers, &periodicLogger{
		id:       id,
		logger:   f,
		interval: b,
		lastRun:  added,
		nextRun:  added.Add(b.NextBackOff()),
	})
	resetTimer(l, added)
}

// RemovePeriodic removes a periodic logger so that it will never be called again.
// If it doesn't exist an error is printed to the logger.
func (l *Logger) RemovePeriodic(id string) {
	l.p.m.Lock()
	defer l.p.m.Unlock()
	n := len(l.p.loggers)
	for i := 0; i < n; i++ {
		if id == l.p.loggers[i].id {
			l.p.loggers[i] = l.p.loggers[n-1] // no-op if last
			l.p.loggers = l.p.loggers[:n-1]
			return
		}
	}
	l.Error("There is no periodic logger with ID %s to remove", id)
}
