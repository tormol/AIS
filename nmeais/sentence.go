// Package nmeais is a library for quickly parsing AIS messages from network packets,
// and merging streams from multiple sources.
package nmeais

import (
	"bytes"
	"fmt" // Only Errorf()
	"time"
)

// ChecksumResult says whether a sentence has a checksum and if it matches
type ChecksumResult byte

// The three valid values of ChecksumResult
const (
	ChecksumPassed = ChecksumResult('t') // The sentence has a chacksum that matches
	ChecksumFailed = ChecksumResult('f') // The sentence has a checksum that doesn't match
	ChecksumAbsent = ChecksumResult('N') // The sentence has no checksum
)

// An untested reimplementation of Nmea183ChecksumCheck:
func checkChecksum(between []byte, hex1, hex2 byte) ChecksumResult {
	hexDigit := func(d byte) byte {
		if d >= '0' && d <= '9' {
			return d - '0'
		}
		if d >= 'A' && d <= 'F' {
			return 10 + d - 'A'
		}
		// lowercase is'nt supported by the standard
		return byte(255)
	}
	first := hexDigit(hex1)
	second := hexDigit(hex2)
	// if the first is >= 8 there is an odd number of non-ASCII characters
	if first < 8 && second < 16 {
		sum := (first << 4) | second
		for _, b := range between {
			sum ^= b
		}
		if sum == 0 {
			return ChecksumPassed
		}
	}
	return ChecksumFailed
}

// Sentence contains the values parsed from a NMEA 0183 sentence assumed to
// encapsulate an AIS message, and the sentence itself.
// Saves all possibly interesting information; some of them are never actually used for anything.
// There are many fields, but most of them are small: Text takes up nearly half the size.
type Sentence struct {
	Identifier   [5]byte // "AIVDM" and the like
	Parts        uint8   // starts at 1
	PartIndex    uint8   // starts at 0
	SMID         uint8   // Sequential message ID, 10 when missing (10 makes indexing based on it easy)
	HasSMID      bool    // Is false if SMID field is empty
	Channel      byte    // '*' if empty
	padding      uint8
	Checksum     ChecksumResult
	payloadStart uint16 // .Text[.payloadStart:.payloadEnd]
	payloadEnd   uint16
	Received     time.Time
	Text         string // everything plus "\r\n"
}

// Payload returns a view of the ASCII-armored payload
// plus how many bits of the last character should be ignored.
func (s Sentence) Payload() (string, uint8) {
	return s.Text[s.payloadStart:s.payloadEnd], s.padding
}

// ParseSentence extracts the fields out of an assumed NMEA0183 AIS-containing sentence.
// It does the minimum possible validation for the sentence to be useful:
// All fields (except Received) might contain invalid values, call .Validate() to check them.
// The checksum is evaluated if present, but not even a checksum mismatch is an error;
// the result is stored in .Checksum.
// For speed, ParseSentence assumes the correct width of fixed-width fields.
func ParseSentence(b []byte, received time.Time) (Sentence, error) {
	if len(b) < 17 /* len("!AIVDM,1,1,,,0,2\r\n") */ {
		return Sentence{}, fmt.Errorf("too short (%d bytes)", len(b))
	}
	if len(b) >= 9*82 {
		// The reference says 82 is maximum, but I frequently get longer, even
		// longer than 255, so increase the limit to something that shouldn't be
		// reached by any malformed encoding of a valid AIS message.
		return Sentence{}, fmt.Errorf("too long (%d bytes)", len(b))
	}
	s := Sentence{
		Text:         string(b),
		Received:     received,
		Identifier:   [5]byte{b[1], b[2], b[3], b[4], b[5]},
		Parts:        b[7] - '0',
		PartIndex:    b[9] - '1',
		SMID:         10,
		HasSMID:      false,
		Channel:      '*',
		payloadStart: 255,
		payloadEnd:   0,
		padding:      255,
		Checksum:     ChecksumAbsent,
	}

	empty := 0
	smid := b[11]
	channel := b[13] // A or B, or 1 or 2, or empty
	if smid != ',' {
		s.SMID = smid - '0'
		s.HasSMID = true
	} else {
		empty++
		channel = b[13-empty]
	}
	if channel != ',' {
		s.Channel = channel
	} else {
		empty++
	}

	payloadStart := 15 - empty
	payloadLen := bytes.IndexByte(b[payloadStart:], ',')
	if payloadLen == -1 {
		return s, fmt.Errorf("too few commas")
	}
	// allow empty payload in case the sentence completes a message
	lastComma := payloadStart + payloadLen
	s.payloadStart = uint16(payloadStart)
	s.payloadEnd = uint16(lastComma)
	after := len(b) - 2 /*CRLF*/ - (lastComma + 1)
	switch after {
	case 4:
		hex1, hex2 := b[lastComma+3], b[lastComma+4]
		s.Checksum = checkChecksum(b[1:lastComma+2], hex1, hex2)
		// a message with a failed checksum might be used to discard an
		// existing incomplete message
		fallthrough
	case 1:
		s.padding = b[lastComma+1] - '0'
		return s, nil
	default:
		return s, fmt.Errorf("error in padding or checksum (%d characters after payload)", after)
	}
}

// Validate performs many checks that ParseSentence doesn't.
func (s Sentence) Validate(parserErr error) error {
	identifiers := [...]string{
		"ABVD", "ADVD", "AIVD", "ANVD", "ARVD",
		"ASVD", "ATVD", "AXVD", "BSVD", "SAVD",
	} // last is M for over the air, O from ourself/ownship (kystverket transmits a few of those)
	if parserErr != nil {
		return parserErr
	}
	valid := false
	for _, id := range identifiers {
		if string(s.Identifier[:4]) == id {
			valid = true
			break
		}
	}
	if !valid || (s.Identifier[4] != 'M' && s.Identifier[4] != 'O') {
		return fmt.Errorf("unrecognized identifier: %s", s.Identifier)
	} else if s.Parts > 9 || s.Parts == 0 {
		return fmt.Errorf("parts is not a positive digit")
	} else if s.PartIndex >= s.Parts { // only used if parts != 1
		return fmt.Errorf("part is not a digit or too high")
	} else if s.HasSMID && s.SMID > 9 { // only used if parts != 1
		return fmt.Errorf("SMID is not a digit but %c", s.SMID+'0')
	} else if s.padding > 5 { // sometimes 6, only used for messages we want to decode
		return fmt.Errorf("padding is not a digit but %c", s.padding+'0')
	} else if !s.HasSMID && s.Parts != 1 { // pretty common
		return fmt.Errorf("multipart message without SMID")
	} else if s.HasSMID && s.Parts == 1 { // pretty common
		return fmt.Errorf("standalone sentence with SMID")
	} else if s.Channel != 'A' && s.Channel != 'B' {
		if s.Channel == '1' || s.Channel == '2' {
			s.Channel = s.Channel - '1' + 'A'
		} else if s.Channel != '*' {
			return fmt.Errorf("unrecognized channel: %c", s.Channel)
		}
	}
	empty, emptySmid := 0, 0
	if !s.HasSMID {
		empty++
		emptySmid++
	}
	if s.Channel == '*' {
		empty++
	}

	// The parser doesn't check if there is a comma when the preceeding value is fixed width.
	if s.Text[0] != '!' {
		return fmt.Errorf("expected '!' as first byte, got '%c'", s.Text[0])
	} else if s.Text[len(s.Text)-2:] != "\r\n" {
		return fmt.Errorf("expected \"\r\n\" at end, got \"%s\"",
			s.Text[len(s.Text)-2:])
	}
	lastComma := len(s.Text) - 4
	if s.Checksum != ChecksumAbsent {
		lastComma = len(s.Text) - 7
		if s.Text[len(s.Text)-5] != '*' {
			return fmt.Errorf("expected '*' at index -5, go '%c'",
				s.Text[len(s.Text)-5])
		}
	}
	for _, at := range []int{6, 8, 10, 12 - emptySmid, 14 - empty, lastComma} {
		if s.Text[at] != ',' {
			return fmt.Errorf("expected ',' at index %d, got '%c'",
				at, s.Text[at])
		}
	}
	return nil
}
