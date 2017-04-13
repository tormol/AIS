package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level is the importance of a logged event
type Level uint8

// log message importance
const (
	Debug   Level = iota // passed through without prepending timestamp
	Fatal                // irrecoverable error
	Error                // non-fatal but permanent degradation
	Warning              // temporary degradation or transient IO error or client error
	Info                 // unimportant but noteworthy
	Ignore               // don't print
)

// initalInterval allows the periodic loggers to be ran soon after start to show whether everything is working.
const initalInterval = 2 * time.Second

// fatalExitCode is the code Logger will abort the process with if a fatal-level message is printed
const fatalExitCode int = 3

// Logger is an utility for thread-safe and periodic logging.
// Use .Log() or one of its wrappers for issues that can be caught as they happen,
// PeriodicLogger for statistics and.
// Use .Compose() to make sure multi-statement messages get written as one.
// Should not be dereferenced or moved as it contains mutexes
type Logger struct {
	writeTo   io.WriteCloser
	writeLock sync.Mutex
	Treshold  Level
	p         periodic
}

// NewLogger creates a new logger with a minimum importance level and the interval to check the periodic loggers
// Even though Logger implements WriteCloser, Loggers should not be nested.
func NewLogger(writeTo io.WriteCloser, treshold Level) *Logger {
	l := &Logger{
		p:         newPeriodic(),
		writeLock: sync.Mutex{},
		writeTo:   writeTo,
		Treshold:  treshold,
	}
	go periodicRunner(l)
	return l
}

// Close the underlying Writer
func (l *Logger) Close() {
	l.writeLock.Lock()
	l.p.Close()
	// Might return an error, but where should the error message be written?
	_ = l.writeTo.Close()
	l.writeTo = nil
	// Dereferencing a nil is better than silently waiting forever on a lock.
	l.writeLock.Unlock()
}

func (l *Logger) prefixMessage(level Level) {
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
func (l *Logger) Compose(level Level) Composer {
	if level > l.Treshold {
		return Composer{
			writeTo:  nil,
			heldLock: nil,
			fatal:    false,
		}
	}
	l.writeLock.Lock()
	l.prefixMessage(level)
	return Composer{
		writeTo:  l.writeTo,
		heldLock: &l.writeLock,
		fatal:    level == Fatal,
	}
}

// Log writes the message if it passes the loggers importance treshold
func (l *Logger) Log(level Level, format string, args ...interface{}) {
	if level <= l.Treshold {
		l.writeLock.Lock()
		defer l.writeLock.Unlock()
		l.prefixMessage(level)
		if len(args) == 0 {
			fmt.Fprintln(l.writeTo, format)
		} else {
			fmt.Fprintf(l.writeTo, format, args...)
			fmt.Fprintln(l.writeTo)
		}
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
func (l *Logger) WriteAdapter(level Level) io.Writer {
	if level > l.Treshold {
		return nil // faster and uses less memory
	}
	return &writeAdapter{
		logger: l,
		level:  level,
	}
}

// Wrappers around Log()

// Debug prints possibly interesting information, and is never filtered.
// Calls to this should probably not be committed.
func (l *Logger) Debug(format string, args ...interface{}) {
	l.Log(Debug, format, args...)
}

// Info prints unimportant but noteworthy events or information
func (l *Logger) Info(format string, args ...interface{}) {
	l.Log(Info, format, args...)
}

// Warning prints an error that might be recovered from
func (l *Logger) Warning(format string, args ...interface{}) {
	l.Log(Warning, format, args...)
}

// Error prints a non-fatal but permanent error
func (l *Logger) Error(format string, args ...interface{}) {
	l.Log(Error, format, args...)
}

// Fatal prints an error message and exits the program
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
	fatal    bool
	writeTo  io.Writer // nil if level is ignored
	heldLock *sync.Mutex
}

// Write writes formatted text without a newline
func (c *Composer) Write(format string, args ...interface{}) {
	if c.writeTo != nil {
		if len(args) == 0 {
			fmt.Fprint(c.writeTo, format)
		} else {
			fmt.Fprintf(c.writeTo, format, args...)
		}
	}
}

// Writeln writes a formatted string plus a newline.
// This is identical to what Logger.Log() does.
func (c *Composer) Writeln(format string, args ...interface{}) {
	if c.writeTo != nil {
		if len(args) == 0 {
			fmt.Fprintln(c.writeTo, format)
		} else {
			fmt.Fprintf(c.writeTo, format, args...)
			fmt.Fprintln(c.writeTo)
		}
	}
}

// Finish writes a formatted line and then closes the composer.
func (c *Composer) Finish(format string, args ...interface{}) {
	c.Writeln(format, args...)
	c.Close()
}

// Close releases the mutex on the logger and exits the process for `Fatal` errors.
func (c *Composer) Close() {
	if c.writeTo != nil {
		c.heldLock.Unlock()
		c.writeTo = nil
		if c.fatal {
			os.Exit(fatalExitCode)
		}
	}
}

// See documentation on WriteAdapter
type writeAdapter struct {
	logger *Logger
	buf    []byte
	level  Level
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
