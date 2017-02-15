package main

import (
	"bytes"
	"fmt" // Only Errorf()
	ais "github.com/andmarios/aislib"
	"sync/atomic"
	"time"
	// "encoding/hex"
)

const (
	MAX_MULTI_TIMESPAN = 2 * time.Second
)

// Saves all possibly interesting information.
// Many fields, but most of them are small: text takes up half the size!
type Sentence struct {
	identifier     [5]byte
	parts          uint8
	part_index     uint8
	smid_i         uint8 // Sequential message ID, 10 if empty
	smid_b         byte  // '*' if empty
	channel        byte  // '*' if empty
	padding        uint8
	checksumPassed bool
	payloadStart   uint8 // .text[.payload_start:payload_end]
	payloadEnd     uint8
	received       time.Time
	text           []byte // everything, for forwarding
}

// Does the minimum possible validation for the sentence to be useful
func parseSentence(b []byte, received time.Time) (Sentence, error) {
	if len(b) < 17 /* len("!AIVDM,1,1,,,0,2\r\n") */ {
		return Sentence{}, fmt.Errorf("too short (%d bytes)", len(b))
	}
	if len(b) > 99 /* 82, but I frequently get 86-byte sentences from ECC*/ {
		return Sentence{}, fmt.Errorf("too long (%d bytes)", len(b))
	}
	s := Sentence{
		text:           b,
		received:       received,
		identifier:     [5]byte{b[1], b[2], b[3], b[4], b[5]},
		parts:          uint8(b[7] - byte('0')),
		part_index:     uint8(b[9] - byte('1')),
		smid_i:         10,
		smid_b:         byte('*'),
		channel:        byte('*'),
		payloadStart:   255,
		payloadEnd:     0,
		padding:        255,
		checksumPassed: true,
	}

	empty := 0
	smid := b[11]
	channel := b[13] // A or B, or 1 or 2, or empty ignore
	if smid != byte(',') {
		s.smid_b = smid
		s.smid_i = uint8(smid - byte('0'))
	} else {
		empty++
		channel = b[13-empty]
	}
	if channel != byte(',') {
		s.channel = channel
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
	if s.parts > 9 || s.parts == 0 {
		return s, fmt.Errorf("parts is not a digit")
	} else if s.part_index >= s.parts {
		return s, fmt.Errorf("part is n")
	} else if s.smid_i > 10 {
		return s, fmt.Errorf("smid is not a digit but %c", s.smid_b)
	} else if s.padding > 5 {
		return s, fmt.Errorf("padding is not a digit but %c", s.padding)
	} else if after == 1 {
		return s, nil // no checksum
	} else if after != 4 {
		return s, fmt.Errorf("error in padding or checksum (len: %d)", after)
	}

	s.checksumPassed = ais.Nmea183ChecksumCheck(string(b[:lastComma+5]))
	return s, nil
	// sum := 0
	// for _,b := b[1:lastComma+1] {
	// 	sum ^= b
	// }
	// checksum := [1]byte{0}
	// _, err := hex.Decode(checksum[:], b[lastComma+3 : lastComma+5])
	// s.checksumPassed = sum == checsum
	// return s, err
}

// For debugging my assumptions
func (s Sentence) validate(parse_err error) error {
	identifiers := []string{
		"ABVD", "ADVD", "AIVD", "ANVD", "ARVD",
		"ASVD", "ATVD", "AXVD", "BSVD", "SAVD",
	} // last is M for over the air, O from ourself (kystverket transmits a few of those)
	if parse_err != nil {
		return parse_err
	}
	valid := false
	for _, id := range identifiers {
		if string(s.identifier[:4]) == id {
			valid = true
			break
		}
	}
	if !valid || (s.identifier[4] != byte('M') && s.identifier[4] != byte('O')) {
		return fmt.Errorf("Unrecognized identifier: %s", s.identifier)
		// commented out because they happen frequently
		// } else if s.smid_t == byte('*') && s.parts != 1 {
		// 	return fmt.Errorf("multipart message without smid")
		// } else if s.smid_t != byte('*') && s.parts == 1 {
		// 	return fmt.Errorf("standalone sentence with smid")
	} else if s.smid_b != byte('*') && s.smid_i == 10 {
		return fmt.Errorf("smid is kinda wrong: %c", s.smid_b)
	} else if s.channel != byte('A') && s.channel != byte('B') {
		if s.channel == byte('1') || s.channel == byte('2') {
			s.channel = s.channel - byte('1') + byte('A')
		} else if s.channel != byte('*') {
			return fmt.Errorf("Unrecognized channel: %c", s.channel)
		}
	} else if s.padding < byte('0') || s.padding > byte('9') {
		return fmt.Errorf("padding is not a number but %c", s.padding)
	}
	emptySmid := 0
	if s.smid_b == byte('*') {
		emptySmid = 1
	}
	empty := emptySmid
	if s.channel == byte('*') {
		empty++
	}
	for n, at := range []int{0, 6, 8, 10, 12 - emptySmid, 14 - empty, -7, -5, -2, -1} {
		expect := []byte("!,,,,,,*\r\n")[n]
		if at < 0 {
			at += len(s.text)
		}
		if s.text[at] != expect {
			return fmt.Errorf("Expected '%c' at index %d, got '%c'. (channel: %c)",
				expect, at, s.text[at], s.channel)
		}
	}
	return nil
}

// Returns a view of the ASCII-armored payload
func (s Sentence) Payload() []byte {
	return s.text[s.payloadStart:s.payloadEnd]
}

type incompleteMessage struct {
	sentences [11]Sentence
	have      uint16 // bit field
	parts     uint8
	missing   uint8
	started   time.Time
}

// Merges multi-sentence messages.
// Sentences can come out of order,
// and messages with different smid can come intertwined
// single-sentence messages fly through without affecting multipart messages.
type multiMerger [11]incompleteMessage

func newMultiMerger() multiMerger {
	return multiMerger([11]incompleteMessage{})
}
func (mm *multiMerger) reset(smid uint8) {
	mm[smid].have = 0
	mm[smid].missing = 0
}
func (mm *multiMerger) restart_with(s Sentence) {
	mm[s.smid_i].sentences[s.part_index] = s
	mm[s.smid_i].started = s.received
	mm[s.smid_i].have = 1 << s.part_index
	mm[s.smid_i].parts = s.parts
	mm[s.smid_i].missing = s.parts - 1
}

// s should not have failed the checksum
func (mm *multiMerger) accept(s Sentence) ([]Sentence, error) {
	if s.parts < 2 {
		return []Sentence{s}, nil
	} else if mm[s.smid_i].missing == 0 {
		mm.restart_with(s)
		return nil, nil
	} else if mm[s.smid_i].parts != s.parts {
		mm.restart_with(s)
		return nil, fmt.Errorf("Out of order")
	} else if mm[s.smid_i].have&(1<<s.part_index) != 0 {
		mm.restart_with(s)
		return nil, fmt.Errorf("Already got")
	} else if s.received.Sub(mm[s.smid_i].started) >= MAX_MULTI_TIMESPAN {
		mm.restart_with(s)
		return nil, fmt.Errorf("Too old")
	} else {
		mm[s.smid_i].sentences[s.part_index] = s
		mm[s.smid_i].have |= 1 << s.part_index
		mm[s.smid_i].missing--
		if mm[s.smid_i].missing == 0 {
			buf := make([]Sentence, 0, s.parts)
			copied := append(buf, mm[s.smid_i].sentences[:s.parts]...)
			return copied, nil
		} else {
			return nil, nil
		}
	}
}

// invalidate message if one that failed the checksum has the same smid and part,
// and the part index isn't already received.
func (mm *multiMerger) reject(s Sentence) bool {
	if s.parts < 2 ||
		mm[s.smid_i].missing == 0 ||
		mm[s.smid_i].parts != s.parts ||
		s.received.Sub(mm[s.smid_i].started) >= MAX_MULTI_TIMESPAN ||
		mm[s.smid_i].have&(1<<s.part_index) != 0 {
		return false
	} else {
		mm.reset(s.smid_i)
		return true
	}
}

type sendSentence struct {
	received time.Time
	text     []byte
}

// splits and merges packets into sentences
type PacketParser struct {
	incomplete []byte
	async      chan sendSentence
	gotFull    uint32
	sourceName string
}

func (pp *PacketParser) decodeSentences(out chan<- Message) {
	mm := newMultiMerger()
	ok := 0
	logbad := func(source []byte, why string) {
		c := AisLog.Compose(LOG_DEBUG)
		if ok != 0 {
			c.Writeln("...%d ok...", ok)
			ok = 0
		}
		c.Writeln(Escape(source))
		c.Finish(why)
	}
	for sentence := range pp.async {
		// fmt.Print(string(sentence.text))
		s, err := parseSentence(sentence.text, sentence.received)
		// err = s.validateSentence(err)
		if err != nil {
			logbad(sentence.text, err.Error())
			continue
		}
		if !s.checksumPassed {
			logbad(s.text, "Checksum failed")
			mm.reject(s)
			continue
		}
		ok++
		sentences, err := mm.accept(s)
		if err != nil {
			AisLog.Debug("multisentence message ejectted for %s", err.Error())
		} else if len(sentences) != 0 {
			msg := Message{
				completed: sentences[len(sentences)-1].received,
				sentences: sentences,
				source:    pp.sourceName,
				msg_type:  sentences[0].Payload()[0],
			}
			out <- msg
		}
	}
}

/* The synchronous side */

func NewPacketParser(source string, dst chan<- Message) *PacketParser {
	pp := &PacketParser{
		async:      make(chan sendSentence, 200),
		sourceName: source,
	}
	go pp.decodeSentences(dst)
	return pp
}
func (pp *PacketParser) Close() {
	close(pp.async)
}
func (pp *PacketParser) Log(l *Logger) {
	full := atomic.SwapUint32(&pp.gotFull, 0) != 0
	l.Info("\tin channel: %d/%d, got full: %t",
		len(pp.async), cap(pp.async), full)
}

// bufferSlice cannot be sent to buffered channels: slicing doesn't copy.
func (pp *PacketParser) Accept(bufferSlice []byte, received time.Time) {
	if len(pp.incomplete) != 0 {
		bufferSlice = append(pp.incomplete, bufferSlice...)
		pp.incomplete = []byte{}
	}
	for len(bufferSlice) != 0 {
		s, incomplete, remaining := pp.splitPacket(bufferSlice)
		// AisLog.Info("%s -> (%s,%s)", Escape(bufferSlice), Escape(s), Escape(remaining))
		bufferSlice = remaining
		if incomplete {
			pp.incomplete = s
			return
		}
		pp.send(s, received)
	}
}
func (pp *PacketParser) send(sentence []byte, received time.Time) {
	if len(pp.async) == cap(pp.async) {
		atomic.StoreUint32(&pp.gotFull, 1)
	}
	pp.async <- sendSentence{
		received: received,
		text:     sentence,
	}
}

// returns (copiedSentence, incomplete, remaining)
func (pp *PacketParser) splitPacket(bufferSlice []byte) ([]byte, bool, []byte) {
	start := bytes.IndexByte(bufferSlice, byte('!'))
	if start > 0 {
		AisLog.Debug("%s\nPacket doesn't start with '!'", Escape(bufferSlice))
		bufferSlice = bufferSlice[start:]
	} else if start < 0 {
		AisLog.Debug("%s\nNo sentence in packet", Escape(bufferSlice))
		return []byte{}, false, []byte{}
	}
	nextm1 := bytes.IndexByte(bufferSlice[1:], byte('!'))
	end := bytes.IndexByte(bufferSlice, byte('\n'))
	//	AisLog.Debug("nextm1: %d, end: %d", nextm1, end)

	if nextm1 == -1 && end == -1 {
		cp := make([]byte, 0, len(bufferSlice))
		cp = append(cp, bufferSlice...)
		return cp, true, []byte{}
	}
	if end == -1 || (nextm1 != -1 && nextm1+1 < end) { // no newline before next sentence
		cp := make([]byte, 0, nextm1+1)
		cp = append(cp, bufferSlice[:nextm1+1]...)
		cp = append(cp, byte('\r'), byte('\n'))
		return cp, false, bufferSlice[nextm1+1:]
	} else if end == 0 || bufferSlice[end-1] != byte('\r') {
		// ECC uses \n, kystverket uses \r\n, normalize to \r\n for forwarding
		cp := make([]byte, 0, end+2)
		cp = append(cp, bufferSlice[:end]...)
		cp = append(cp, byte('\r'), byte('\n'))
		return cp, false, bufferSlice[end+1:]
	} else {
		cp := make([]byte, 0, end+1)
		cp = append(cp, bufferSlice[:end+1]...)
		return cp, false, bufferSlice[end+1:]
	}
}
