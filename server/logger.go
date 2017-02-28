package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	LOG_DEBUG   int = 9
	LOG_INFO    int = 7
	LOG_WARNING int = 5
	LOG_ERROR   int = 3
	LOG_FATAL   int = 1
)
const FATAL_EXIT_CODE int = 3

type loggerFunc func(l *Logger, sinceLast time.Duration)

type periodicLogger struct {
	id          string
	minInterval time.Duration
	lastRun     time.Time
	logger      loggerFunc
}

// An utility for thread-safe and periodic logging.
// Use .Log() or one of its wrappers for issues that can be caught as they happen,
// PeriodicLogger for statistics and.
// Use .Compose() to make sure multi-statement messages get written as one.
// Should not be dereferenced or moved as it contains mutexes
type Logger struct {
	writeTo             io.WriteCloser
	writeLock           sync.Mutex
	Level               int
	periodicLoggers     []periodicLogger
	periodicLoggersLock sync.Mutex
	lastWalk            time.Time
	walkInterval        time.Duration
}

func NewLogger(writeTo io.WriteCloser, level int, walkInterval time.Duration) *Logger {
	l := &Logger{
		walkInterval:        walkInterval,
		lastWalk:            time.Now(),
		periodicLoggersLock: sync.Mutex{},
		writeLock:           sync.Mutex{},
		writeTo:             writeTo,
		Level:               level,
	}
	if walkInterval > 0 {
		go func() {
			time.Sleep(l.walkInterval)
			for l.writeTo != nil {
				started := time.Now()
				l.RunPeriodicLoggers(started)
				toSleep := l.walkInterval - time.Since(started)
				time.Sleep(toSleep)
			}
		}()
	}
	return l
}

func (l *Logger) Close() {
	l.writeLock.Lock()
	l.writeTo.Close()
	l.writeTo = nil
	// Dereferencing a nil is better than silently waiting forever on a lock.
	l.writeLock.Unlock()
}

func (l *Logger) AddPeriodicLogger(id string, minInterval time.Duration, f loggerFunc) {
	if l.walkInterval <= 0 {
		l.Error("Cannot add %s because the logger doesn't support periodic loggers.", id)
		return
	}
	l.periodicLoggersLock.Lock()
	defer l.periodicLoggersLock.Unlock()
	for _, c := range l.periodicLoggers {
		if c.id == id {
			Log.Error("A periodic logger with ID %s already exists, there is now a duplicate", c.id)
		}
	}
	l.periodicLoggers = append(l.periodicLoggers, periodicLogger{
		id:          id,
		minInterval: minInterval,
		lastRun:     time.Now(),
		logger:      f,
	})
}

func (l *Logger) RemovePeriodicLogger(id string) {
	l.periodicLoggersLock.Lock()
	defer l.periodicLoggersLock.Unlock()
	for i, c := range l.periodicLoggers {
		if c.id == id {
			n := len(l.periodicLoggers)
			if n-1 > i {
				l.periodicLoggers[i] = l.periodicLoggers[n-1]
			}
			l.periodicLoggers = l.periodicLoggers[:n-1]
			return
		}
	}
	Log.Error("There is no periodic logger with ID %s to remove", id)
}

func (l *Logger) RunPeriodicLoggers(started time.Time) {
	l.periodicLoggersLock.Lock()
	defer l.periodicLoggersLock.Unlock()
	l.lastWalk = started
	for i := range l.periodicLoggers {
		c := &l.periodicLoggers[i] // range returns copies
		d := started.Sub(c.lastRun)
		if d >= c.minInterval {
			c.lastRun = started
			c.logger(l, d)
		}
	}
}

func (l *Logger) prefixMessage(level int) {
	if l.Level < LOG_DEBUG {
		fmt.Fprint(l.writeTo, time.Now().Format("2006-01-02 15:04:05: "))
	}
	if level == LOG_WARNING {
		fmt.Fprint(l.writeTo, "WARNING: ")
	} else if level == LOG_ERROR {
		fmt.Fprint(l.writeTo, "ERROR: ")
	} else if level == LOG_FATAL && l.Level != LOG_DEBUG {
		fmt.Fprint(l.writeTo, "FATAL: ")
	}
}

func (l *Logger) Compose(level int) LogComposer {
	c := LogComposer{
		level:    level,
		writeTo:  nil,
		heldLock: nil,
	}
	if level <= l.Level {
		c.writeTo = l.writeTo
		c.heldLock = &l.writeLock
		l.writeLock.Lock()
		l.prefixMessage(level)
	}
	return c
}

func (l *Logger) Log(level int, format string, args ...interface{}) {
	if level <= l.Level {
		l.writeLock.Lock()
		defer l.writeLock.Unlock()
		l.prefixMessage(level)
		if len(args) == 0 {
			fmt.Fprint(l.writeTo, format)
		} else {
			fmt.Fprintf(l.writeTo, format, args...)
		}
		fmt.Fprintln(l.writeTo)
		if level == LOG_FATAL {
			os.Exit(FATAL_EXIT_CODE)
		}
	}
}

// Wrappers around Log()

func (l *Logger) Debug(format string, args ...interface{}) {
	l.Log(LOG_DEBUG, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.Log(LOG_INFO, format, args...)
}

func (l *Logger) Warning(format string, args ...interface{}) {
	l.Log(LOG_WARNING, format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.Log(LOG_ERROR, format, args...)
}

func (l *Logger) Fatal(format string, args ...interface{}) {
	l.Log(LOG_FATAL, format, args...)
}

func (l *Logger) FatalIf(cond bool, format string, args ...interface{}) {
	if cond {
		l.Fatal(format, args...)
	}
}

func (l *Logger) FatalIfErr(err error, format string, args ...interface{}) {
	if err != nil {
		args = append(args, err.Error())
		l.Fatal("Failed to "+format+": %s", args...)
	}
}

type LogComposer struct {
	level    int       // Only used for LOG_FATAL
	writeTo  io.Writer // nil if level is ignored
	heldLock *sync.Mutex
}

func (l *LogComposer) Write(format string, args ...interface{}) {
	if l.writeTo != nil {
		if len(args) == 0 {
			fmt.Fprint(l.writeTo, format)
		} else {
			fmt.Fprintf(l.writeTo, format, args...)
		}
	}
}

func (l *LogComposer) Writeln(format string, args ...interface{}) {
	if l.writeTo != nil {
		if len(args) == 0 {
			fmt.Fprint(l.writeTo, format)
		} else {
			fmt.Fprintf(l.writeTo, format, args...)
		}
		fmt.Fprintln(l.writeTo)
	}
}

func (l *LogComposer) Finish(format string, args ...interface{}) {
	l.Write(format, args...)
	l.Close()
}

func (l *LogComposer) Close() {
	if l.writeTo != nil {
		fmt.Fprintln(l.writeTo)
		if l.level == LOG_FATAL {
			os.Exit(FATAL_EXIT_CODE)
		}
		l.writeTo = nil
		l.heldLock.Unlock()
	}
}

// A string() that escapes newlines
func Escape(b []byte) string {
	s := make([]byte, 0, len(b))
	for _, c := range b {
		switch c {
		case byte('\r'):
			s = append(s, "\\r"...)
		case byte('\n'):
			s = append(s, "\\n"...)
		case 0:
			s = append(s, "\\0"...)
		default:
			s = append(s, c)
		}
	}
	return string(s)
}

// Round n to the nearest Kilo, Mega, Giga, ..., or Yotta, and append the letter.
// multipleOf can be 1000 or 1024 (or anything >=256 (=(2^64)^(1/8)))
func SiMultiple(n, multipleOf uint64) string {
	var steps, rem uint64
	for n >= multipleOf {
		rem = n % multipleOf
		n /= multipleOf
		steps++
	}
	if rem%multipleOf >= multipleOf/2 {
		n++ // round the last
	}
	s := strconv.FormatUint(n, 10)
	if steps > 0 {
		s += " KMGTPEZY"[steps : steps+1]
	}
	return s
}

// Hide unneeded precission when printing it.
func RoundDuration(d, to time.Duration) string {
	d = d - (d % to)
	return d.String()
}
