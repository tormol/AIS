package nmeais

import (
	"fmt"
	"time"
)

// Message is an AIS message stored as decoded NMEA 0183 sentences.
// It also stores the alias of the source it came from and the time the last part was received.
type Message struct {
	SourceName string     // alias of the AIS listener the message came from
	sentences  []Sentence // one or more AIS sentences
	started    time.Time  // of last received sentence
	ended      time.Time
}

// Sentences returns a slice containing the sentences the message is made up of.
func (m *Message) Sentences() []Sentence {
	return m.sentences[:m.sentences[0].Parts]
}

// Type de-armors only the first byte of the payload.
// This is kinda too high level for this package, but avoids de-armoring the
// whole payload for message types that won't be decoded further.
func (m *Message) Type() uint8 {
	payload, _ := m.sentences[0].Payload()
	return deArmorByte(payload[0])
}

func deArmorByte(b byte) uint8 {
	v := uint8(b) - 48
	if v > 40 {
		v -= 8
	}
	// TODO validation ?
	return v & 0x3f // 0b0011_1111
}

// DearmoredPayload undoes the siz-bit ASCII encoding of the payload.
// This function is completely untested.
func (m *Message) DearmoredPayload() []byte {
	first, _ := m.sentences[0].Payload()
	data := make([]byte, 0, len(first)*8/6)
	bitbuf := uint32(0)
	bits := uint(0)
	for i := range m.sentences {
		payload, _pad := m.sentences[i].Payload()
		for _, b := range []byte(payload) {
			bitbuf = (bitbuf << 6) | uint32(deArmorByte(b))
			bits += 6
			if bits >= 24 { // leave one byte in case it's the last and there is padding
				bits -= 8
				data = append(data, uint8(bitbuf>>bits))
				bits -= 8
				data = append(data, uint8(bitbuf>>bits))
			}
		}
		// TODO validated pad and handle 6
		// if pad > 6 {// FIXME report error and discard message
		// 	return fmt.Errorf("padding is not a digit but %c", byte(s.Padding)+byte('0'))
		// }
		pad := 5 - uint(_pad) // I REALLY doubt this is correct, but esr says so..
		bits -= pad
		bitbuf >>= pad
	}
	for bits >= 8 {
		bits -= 8
		data = append(data, uint8(bitbuf>>bits))
	}
	return data
}

// ArmoredPayload joins together the payload part of the sentences the message was parsed from.
func (m *Message) ArmoredPayload() string {
	if len(m.sentences) == 1 {
		first, _ := m.sentences[0].Payload()
		return first
	}
	combined := ""
	for i := range m.sentences {
		payload, _ := m.sentences[i].Payload()
		combined += payload
	}
	return combined
}

// Text joins together the sentences that the message was created from.
// A newline is inserted after every sentence.
func (m *Message) Text() string {
	if len(m.sentences) == 1 {
		return m.sentences[0].Text
	}
	combined := ""
	for i := range m.sentences {
		combined += m.sentences[i].Text
	}
	return combined
}

// An incomplete message with a certain SMID.
// The SMID itself is not stored because it's the kay and this is the value.
// The struct is big, but it'r reused.
type incompleteMessage struct {
	sentences [9]Sentence // longest message takes 4-5 sentences, 9 for future-proofing
	have      uint16      // bit field: if least significant bit N is set, sentence with PartIndex N is received
	parts     uint8       // != sentences[0] parts because [0] might not have been received
	missing   uint8       // = parts - number of bits set in have
	started   time.Time   // THe time of the first received part is the closest to when it was sent
}

// MessageAssembler takes in sentences out of order and
// returns a Message if the sentence completes one.
// Sentences can come out of order, as can messages with different SMID.
// Single-sentence messages pass through without affecting multi-sentence messages.
type MessageAssembler struct {
	incomplete         [11]incompleteMessage
	MaxMessageTimespan time.Duration
	SourceName         string
}

// NewMessageAssembler creates a new MessageAssembler.
// There's nothing happening behind the scenes, so a value is returned,
// but the struct is quite big so it shouldn't be moved around too much.
func NewMessageAssembler(maxMessageTimespan time.Duration, sourceName string) MessageAssembler {
	return MessageAssembler{
		incomplete:         [11]incompleteMessage{},
		MaxMessageTimespan: maxMessageTimespan,
		SourceName:         sourceName,
	}
}

// Forget any existing sentences with this SMID
func (ma *MessageAssembler) reset(smid uint8) {
	ma.incomplete[smid].have = 0
	ma.incomplete[smid].missing = 0
}

// Reuse the SMID of s for a new message of which s is a part.
func (ma *MessageAssembler) restartWith(s Sentence) {
	ma.incomplete[s.SMID].sentences[s.PartIndex] = s
	ma.incomplete[s.SMID].started = s.Received
	ma.incomplete[s.SMID].have = 1 << s.PartIndex
	ma.incomplete[s.SMID].parts = s.Parts
	ma.incomplete[s.SMID].missing = s.Parts - 1
}

// Accept takes in a sentence, returns a Message if it completes one,
// an error if it's invalid or aborts an incomplete one, or neither.
// Sentences that have failed the checksum are checked against incomplete messages,
// and if it matches the message is aborted.
func (ma *MessageAssembler) Accept(s Sentence) (*Message, error) {
	if s.Checksum == ChecksumFailed {
		err := "Checksum failed"
		if ma.abortSMID(s) {
			err += " and an incomplete message dropped"
		}
		return nil, fmt.Errorf(err)
	} else if s.Parts < 2 {
		return &Message{
			sentences:  []Sentence{s},
			SourceName: ma.SourceName,
			started:    s.Received,
			ended:      s.Received,
		}, nil
	} else if s.PartIndex >= s.Parts {
		return nil, fmt.Errorf("part number is not a digit or too high")
	} else if s.SMID > 10 {
		return nil, fmt.Errorf("SMID is not a digit but %c", byte(s.SMID)+byte('0'))
	} else if ma.incomplete[s.SMID].missing == 0 {
		ma.restartWith(s)
		return nil, nil
	} else if s.Received.Sub(ma.incomplete[s.SMID].started) >= ma.MaxMessageTimespan {
		ma.restartWith(s)
		return nil, fmt.Errorf("Too old")
	} else if ma.incomplete[s.SMID].parts != s.Parts {
		ma.restartWith(s)
		return nil, fmt.Errorf("SMID collision of out-of-order messages")
	} else if ma.incomplete[s.SMID].have&(1<<s.PartIndex) != 0 {
		ma.restartWith(s)
		return nil, fmt.Errorf("Already got")
	} else {
		ma.incomplete[s.SMID].sentences[s.PartIndex] = s
		ma.incomplete[s.SMID].have |= 1 << s.PartIndex
		ma.incomplete[s.SMID].missing--
		if ma.incomplete[s.SMID].missing == 0 {
			return &Message{
				sentences:  append([]Sentence{}, ma.incomplete[s.SMID].sentences[:s.Parts]...),
				SourceName: ma.SourceName,
				started:    ma.incomplete[s.SMID].started,
				ended:      s.Received,
			}, nil
		}
		return nil, nil
	}
}

// Invalidate message if one that failed the checksum has the same SMID and part,
// and the part index haven't already been received.
func (ma *MessageAssembler) abortSMID(s Sentence) bool {
	if s.Parts < 2 ||
		s.SMID > 10 ||
		ma.incomplete[s.SMID].missing == 0 ||
		ma.incomplete[s.SMID].parts != s.Parts ||
		s.Received.Sub(ma.incomplete[s.SMID].started) >= ma.MaxMessageTimespan ||
		ma.incomplete[s.SMID].have&(1<<s.PartIndex) != 0 {
		return false
	}
	ma.reset(s.SMID)
	return true
}
