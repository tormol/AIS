package logger

// Functions and structs that are unrelated to Logger
// but are used for formatting values for logging.
import (
	"strconv"
	"time"
)

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
