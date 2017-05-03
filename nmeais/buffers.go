package nmeais

// Code for splitting network packets or disk reads into AIS NMEA sentences.
// Because it only looks for '!' and not '$' it cannot be used for non-AIS sentences.
// While adding a parameter for which byte to look for is trivial,
// users might also want to look for both, so we do neither since we don't need it.
import (
	"bytes"
)

// FirstSentenceInBuffer extracts the text of what looks like the first AIS NMEA0183 sentence
// in an (IO) buffer.
// `next` is the index of the first byte that wasn't copied,
// it is len(bufferSlice) if the entire input was used.
// Otherwise it's ensured that `copiedSentence`` ends with a "\r\n" line delimiter.
// Bytes before the first '!' are considered noise and skipped.
// This newline fixing and '!'-seeking means that `next` might be different from
// len(copiedSentence)-len(incomplete).
// The sentence is always copied so that the input buffer can be reused immediately.
// If the buffer doesn't contain a complete sentence, a copy of the input is returned as
// `copiedSentence` and `next` is -1.
// If the buffer doesn't even contain the start of a sentence, `"",-1` is returned to not create
// an edge case where `used` is positive but `copiedSentence` is empty.
// `incomplete` is a receptacle for that copy: if it's non-empty it's prepended to `copiedSentence`
// and the search for a starting '!' is dropped.
func FirstSentenceInBuffer(incomplete, bufferSlice []byte) (copiedSentence []byte, next int) {
	next = -1
	if len(incomplete) == 0 {
		start := bytes.IndexByte(bufferSlice, '!')
		if start == -1 {
			return []byte{}, -1
		}
		bufferSlice = bufferSlice[start:]
		// start search after the '!' at index 0
		nextm1 := bytes.IndexByte(bufferSlice[1:], '!') // next minus one
		if nextm1 != -1 {
			next = nextm1 + 1
		}
	} else {
		// do look at the first byte; if the connection was restarted, the server might have
		// forgotten how fa the clien was and started sending a new sentence.
		// `incomplete` might just have been missing a newline,
		// so return it even if it will likely be invalid.
		next = bytes.IndexByte(bufferSlice, '!')
	}

	end := bytes.IndexByte(bufferSlice, '\n')

	if next == -1 && end == -1 { // incomplete sentence
		return append(incomplete, bufferSlice...), -1
	} else if end == -1 || (next != -1 && next < end) { // no newline before next sentence
		// cpy = copy but not a builtin
		cpy := reserveCapacity(incomplete, next+2)
		cpy = append(cpy, bufferSlice[:next]...)
		cpy = append(cpy, '\r', '\n')
		return cpy, next
	} else if (end != 0 && bufferSlice[end-1] == '\r') ||
		(end == 0 && len(incomplete) != 0 && incomplete[len(incomplete)-1] == '\r') {
		return append(incomplete, bufferSlice[:end+1]...), end + 1 // Both \r and \n
	} else { // only \n, normalize to \r\n for consistency
		cpy := reserveCapacity(incomplete, end+2)
		cpy = append(cpy, bufferSlice[:end]...)
		cpy = append(cpy, '\r', '\n')
		return cpy, end + 1 // consume the newline even though it wasn't used
	}
}

func reserveCapacity(b []byte, add int) []byte {
	if cap(b) >= len(b)+add {
		return b
	}
	return append(make([]byte, 0, len(b)+add), b...)
}
