package logger

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

// log message importance
const (
	Debug   int = 9 // temporary or possibly interesting
	Info    int = 7 // interesting
	Warning int = 5 // temporary or client error
	Error   int = 3 // permanent degradation
	Fatal   int = 1 // irrecoverable error
)

// initalInterval allows the periodic loggers to be ran soon after start to show whether everything is working.
const initalInterval = 2 * time.Second

// fatalExitCode is the code Logger will abort the process with if a fatal-level message is printed
const fatalExitCode int = 3

type loggerFunc func(l *Logger, sinceLast time.Duration)

type periodicLogger struct {
	id          string
	minInterval time.Duration
	lastRun     time.Time
	logger      loggerFunc
}

// Logger is an utility for thread-safe and periodic logging.
// Use .Log() or one of its wrappers for issues that can be caught as they happen,
// PeriodicLogger for statistics and.
// Use .Compose() to make sure multi-statement messages get written as one.
// Should not be dereferenced or moved as it contains mutexes
type Logger struct {
	writeTo             io.WriteCloser
	writeLock           sync.Mutex
	Treshold            int
	periodicLoggers     []periodicLogger
	periodicLoggersLock sync.Mutex
	lastWalk            time.Time
	walkInterval        time.Duration
}

// NewLogger creates a new logger with a minimum importance level and the interval to check the periodic loggers
// Even though Logger implements WriteCloser, Loggers should not be nested.
func NewLogger(writeTo io.WriteCloser, level int, walkInterval time.Duration) *Logger {
	l := &Logger{
		walkInterval:        walkInterval,
		lastWalk:            time.Now(),
		periodicLoggersLock: sync.Mutex{},
		writeLock:           sync.Mutex{},
		writeTo:             writeTo,
		Treshold:            level,
	}
	if walkInterval > 0 {
		go func() {
			time.Sleep(initalInterval) // Show that everything is working
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

// Close the underlying Writer
func (l *Logger) Close() {
	l.writeLock.Lock()
	// Might return an error, but where should the error message be written?
	_ = l.writeTo.Close()
	l.writeTo = nil
	// Dereferencing a nil is better than silently waiting forever on a lock.
	l.writeLock.Unlock()
}

// AddPeriodicLogger stores a closure that will be called periodically
// The given interval will be rounded up to a multiple of the walkInterval
// the logger was created with. (the list of loggers isn't iterated continiously)
// If the logger doesn't support logging or the interval already exists an error will be printed.
func (l *Logger) AddPeriodicLogger(id string, minInterval time.Duration, f loggerFunc) {
	if l.walkInterval <= 0 {
		l.Error("Cannot add %s because the logger doesn't support periodic loggers.", id)
		return
	}
	l.periodicLoggersLock.Lock()
	defer l.periodicLoggersLock.Unlock()
	for _, c := range l.periodicLoggers {
		if c.id == id {
			l.Error("A periodic logger with ID %s already exists, there is now a duplicate", c.id)
		}
	}
	l.periodicLoggers = append(l.periodicLoggers, periodicLogger{
		id:          id,
		minInterval: minInterval,
		lastRun:     time.Now().Add(-time.Hour),
		logger:      f,
	})
}

// RemovePeriodicLogger removes a periodic logger.
// If it doesn't exist an error is printed.
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
	l.Error("There is no periodic logger with ID %s to remove", id)
}

// RunPeriodicLoggers is exported so main() can call it before exising
func (l *Logger) RunPeriodicLoggers(started time.Time) {
	l.periodicLoggersLock.Lock()
	defer l.periodicLoggersLock.Unlock()
	l.lastWalk = started
	first := true
	for i := range l.periodicLoggers {
		c := &l.periodicLoggers[i] // range returns copies
		d := started.Sub(c.lastRun)
		if d >= c.minInterval {
			if first { // separate runs with a newline
				l.Info("") // TODO pass around a Composer; loggers should run fast enough to not hold up other code.
				first = false
			}
			c.lastRun = started
			c.logger(l, d)
		}
	}
}

func (l *Logger) prefixMessage(level int) {
	if l.Treshold < Debug {
		fmt.Fprint(l.writeTo, time.Now().Format("2006-01-02 15:04:05: "))
	}
	if level == Warning {
		fmt.Fprint(l.writeTo, "WARNING: ")
	} else if level == Error {
		fmt.Fprint(l.writeTo, "ERROR: ")
	} else if level == Fatal && l.Treshold != Debug {
		fmt.Fprint(l.writeTo, "FATAL: ")
	}
}

// Compose allows holding the lock between multiple print
func (l *Logger) Compose(level int) Composer {
	c := Composer{
		level:    level,
		writeTo:  nil,
		heldLock: nil,
	}
	if level <= l.Treshold {
		c.writeTo = l.writeTo
		c.heldLock = &l.writeLock
		l.writeLock.Lock()
		l.prefixMessage(level)
	}
	return c
}

// Log writes the message if it passes the loggers importance treshold
func (l *Logger) Log(level int, format string, args ...interface{}) {
	if level <= l.Treshold {
		l.writeLock.Lock()
		defer l.writeLock.Unlock()
		l.prefixMessage(level)
		if len(args) == 0 {
			fmt.Fprint(l.writeTo, format)
		} else {
			fmt.Fprintf(l.writeTo, format, args...)
		}
		fmt.Fprintln(l.writeTo)
		if level == Fatal {
			os.Exit(fatalExitCode)
		}
	}
}

// WriteAdapter returns a Writer that writes through this logger with the given level.
// Writes that don't end in a newline are buffered to not split messages, but
// Composer-written messages might get split.
// The adapter is not synchronized because both the standard log.Logger and other instances
// of this type serializes writes, and the underlying Logger is synchronized.
func (l *Logger) WriteAdapter(level int) io.Writer {
	if level <= l.Treshold {
		return &writeAdapter{
			logger: l,
			level:  level,
		}
	}
	return nil // faster and uses less memory
}

// Wrappers around Log()

func (l *Logger) Debug(format string, args ...interface{}) {
	l.Log(Debug, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.Log(Info, format, args...)
}

func (l *Logger) Warning(format string, args ...interface{}) {
	l.Log(Warning, format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.Log(Error, format, args...)
}

func (l *Logger) Fatal(format string, args ...interface{}) {
	l.Log(Fatal, format, args...)
}

// FatalIf does nothing if cond is false, but otherwise prints the message and aborts the process.
func (l *Logger) FatalIf(cond bool, format string, args ...interface{}) {
	if cond {
		l.Fatal(format, args...)
	}
}

// FatalIfErr does nothing if err is nil, but otherwise prints "Failed to <..>: $err.Error()" and aborts the process.
func (l *Logger) FatalIfErr(err error, format string, args ...interface{}) {
	if err != nil {
		args = append(args, err.Error())
		l.Fatal("Failed to "+format+": %s", args...)
	}
}

// Composer lets you split a long message into multiple write statements
// End the message by calling Finish() or Close()
type Composer struct {
	level    int       // Only used for Fatal
	writeTo  io.Writer // nil if level is ignored
	heldLock *sync.Mutex
}

// Write writes formatted text without a newline
func (l *Composer) Write(format string, args ...interface{}) {
	if l.writeTo != nil {
		if len(args) == 0 {
			fmt.Fprint(l.writeTo, format)
		} else {
			fmt.Fprintf(l.writeTo, format, args...)
		}
	}
}

// Writeln writes a formatted string plus a newline.
// This is identical to what Logger.Log() does.
func (l *Composer) Writeln(format string, args ...interface{}) {
	if l.writeTo != nil {
		if len(args) == 0 {
			fmt.Fprint(l.writeTo, format)
		} else {
			fmt.Fprintf(l.writeTo, format, args...)
		}
		fmt.Fprintln(l.writeTo)
	}
}

// Finish writes a formatted line and then closes the composer.
func (l *Composer) Finish(format string, args ...interface{}) {
	l.Write(format, args...)
	l.Close()
}

// Close releases the mutex on the logger and exits the process for `Fatal` errors.
func (l *Composer) Close() {
	if l.writeTo != nil {
		fmt.Fprintln(l.writeTo)
		l.heldLock.Unlock()
		if l.level == Fatal {
			os.Exit(fatalExitCode)
		}
		l.writeTo = nil
	}
}

// See documentation on WriteAdapter
type writeAdapter struct {
	logger *Logger
	buf    []byte
	level  int
}

// Always returns len(message), nil
func (wa *writeAdapter) Write(message []byte) (int, error) {
	if len(message) > 0 {
		wa.buf = append(wa.buf, message...)
		if wa.buf[len(wa.buf)-1] == byte('\n') { // flush it
			noTrailingNewline := wa.buf[:len(wa.buf)-1] // Log appends a newline
			wa.logger.Log(wa.level, "%s", string(noTrailingNewline))
			wa.buf = []byte{} // restart
		}
	}
	return len(message), nil
}

// Escape escapes multi-line NMEA sentences for debug logging.
// It replaces CR, LF and NUL with \r, \n and \0,
// and is only slightly longer than string().
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

// SiMultiple rounds n down to the nearest Kilo, Mega, Giga, ..., or Yotta, and append the letter.
// `multipleOf` can be 1000 or 1024 (or anything >=256 (=(2^64)^(1/8))).
// `maxUnit` prevents losing too much precission by using too big units.
func SiMultiple(n, multipleOf uint64, maxUnit byte) string {
	var steps, rem uint64
	units := " KMGTPEZY"
	for n >= multipleOf && units[steps] != maxUnit {
		rem = n % multipleOf
		n /= multipleOf
		steps++
	}
	if rem%multipleOf >= multipleOf/2 {
		n++ // round the last
	}
	s := strconv.FormatUint(n, 10)
	if steps > 0 {
		s += units[steps : steps+1]
	}
	return s
}

// RoundDuration removes excessive precission for printing.
func RoundDuration(d, to time.Duration) string {
	d = d - (d % to)
	return d.String()
}
