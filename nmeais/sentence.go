// Package nmeais is a library for quickly parsing AIS messages from network packets,
// and merging streams from multiple sources.
package nmeais

import (
	"bytes"
	"fmt" // Only Errorf()
	"time"

	ais "github.com/andmarios/aislib"
)

// ChecksumResult says whether a sentence has a checksum and if it matches
type ChecksumResult byte

// The three valid values of ChecksumResult
const (
	ChecksumPassed = ChecksumResult(byte('t')) // The sentence has a chacksum that matches
	ChecksumFailed = ChecksumResult(byte('f')) // The sentence has a checksum that doesn't match
	ChecksumAbsent = ChecksumResult(byte('N')) // The sentence has no checksum
)

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
	payloadStart uint8 // .Text[.payloadStart:.payloadEnd]
	payloadEnd   uint8
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
	if len(b) > 99 /* 82, but I frequently get 86-byte sentences from ECC*/ {
		return Sentence{}, fmt.Errorf("too long (%d bytes)", len(b))
	}
	s := Sentence{
		Text:         string(b),
		Received:     received,
		Identifier:   [5]byte{b[1], b[2], b[3], b[4], b[5]},
		Parts:        uint8(b[7] - byte('0')),
		PartIndex:    uint8(b[9] - byte('1')),
		SMID:         10,
		HasSMID:      false,
		Channel:      byte('*'),
		payloadStart: 255,
		payloadEnd:   0,
		padding:      255,
		Checksum:     ChecksumAbsent,
	}

	empty := 0
	smid := b[11]
	channel := b[13] // A or B, or 1 or 2, or empty
	if smid != byte(',') {
		s.SMID = uint8(smid - byte('0'))
		s.HasSMID = true
	} else {
		empty++
		channel = b[13-empty]
	}
	if channel != byte(',') {
		s.Channel = channel
	} else {
		empty++
	}

	payloadStart := 15 - empty
	payloadLen := bytes.IndexByte(b[payloadStart:], byte(','))
	if payloadLen == -1 {
		return s, fmt.Errorf("too few commas")
	}
	lastComma := payloadStart + payloadLen
	after := len(b) - 2 - (lastComma + 1)
	s.payloadStart = uint8(payloadStart)
	s.payloadEnd = uint8(lastComma)
	s.padding = uint8(b[lastComma+1] - byte('0'))
	if after == 1 {
		return s, nil // no checksum
	} else if after != 4 {
		return s, fmt.Errorf("error in padding or checksum (len: %d)", after)
	}

	if ais.Nmea183ChecksumCheck(string(b[:lastComma+5])) {
		s.Checksum = ChecksumPassed
	} else {
		s.Checksum = ChecksumFailed
	}
	// A message with a failed checksum might be used to discard an existing incomplete message.
	return s, nil
}

// Validate performs many checks that ParseSentence doesn't.
func (s Sentence) Validate(parserErr error) error {
	identifiers := []string{
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
	if !valid || (s.Identifier[4] != byte('M') && s.Identifier[4] != byte('O')) {
		return fmt.Errorf("Unrecognized identifier: %s", s.Identifier)
	} else if s.Parts > 9 || s.Parts == 0 {
		return fmt.Errorf("parts is not a positive digit")
	} else if s.PartIndex >= s.Parts { // only used if parts != 1
		return fmt.Errorf("part is not a digit or too high")
	} else if s.HasSMID && s.SMID > 9 { // only used if parts != 1
		return fmt.Errorf("SMID is not a digit but %c", byte(s.SMID)+byte('0'))
	} else if s.padding > 5 { // sometimes 6, only used for messages we want to decode
		return fmt.Errorf("padding is not a digit but %c", byte(s.padding)+byte('0'))
	} else if !s.HasSMID && s.Parts != 1 { // pretty common
		return fmt.Errorf("multipart message without smid")
	} else if s.HasSMID && s.Parts == 1 { // pretty common
		return fmt.Errorf("standalone sentence with smid")
	} else if s.Channel != byte('A') && s.Channel != byte('B') {
		if s.Channel == byte('1') || s.Channel == byte('2') {
			s.Channel = s.Channel - byte('1') + byte('A')
		} else if s.Channel != byte('*') {
			return fmt.Errorf("Unrecognized channel: %c", s.Channel)
		}
	}
	empty, emptySmid := 0, 0
	if !s.HasSMID {
		empty++
		emptySmid++
	}
	if s.Channel == byte('*') {
		empty++
	}
	// The parser doesn't check if there is a comma when the preceeding value is fixed width.
	for n, at := range []int{0, 6, 8, 10, 12 - emptySmid, 14 - empty, -7, -5, -2, -1} {
		expect := []byte("!,,,,,,*\r\n")[n]
		if at < 0 {
			at += len(s.Text)
		}
		if s.Text[at] != expect {
			return fmt.Errorf("Expected '%c' at index %d, got '%c'. (channel: %c)",
				expect, at, s.Text[at], s.Channel)
		}
	}
	return nil
}