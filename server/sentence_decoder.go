package main

import (
	"bytes"
	"fmt" // Only Errorf()
	ais "github.com/andmarios/aislib"
	"time"
	// "encoding/hex"
)

const (
	MAX_MULTI_TIMESPAN = 2 * time.Second
)

// Contains the values parsed from a NMEA0183 sentence assumed to encapsulate an
// AIS message, and the sentence itself.
// Saves all possibly interesting information, some of them are never actually used for anything.
// Many fields, but most of them are small: text takes up narly half the size.
type Sentence struct {
	identifier     [5]byte // "AIVDM" and the like
	parts          uint8   // starts at 1
	part_index     uint8   // starts at 0
	smid_i         uint8   // Sequential message ID, 10 if empty
	smid_b         byte    // '*' if empty
	channel        byte    // '*' if empty
	Padding        uint8
	checksumPassed bool
	payloadStart   uint8 // .Text[.payload_start:payload_end]
	payloadEnd     uint8
	Received       time.Time
	Text           []byte // everything, for forwarding
}

// Extracts the fields out of an assumed NMEA0183, AIS-containing sentence.
// Does the minimum possible validation for the sentence to be useful;
// For speed, it assumes the correct with of fixed-width fielsd,
// and doesn't actually split by comma.
func parseSentence(b []byte, received time.Time) (Sentence, error) {
	if len(b) < 17 /* len("!AIVDM,1,1,,,0,2\r\n") */ {
		return Sentence{}, fmt.Errorf("too short (%d bytes)", len(b))
	}
	if len(b) > 99 /* 82, but I frequently get 86-byte sentences from ECC*/ {
		return Sentence{}, fmt.Errorf("too long (%d bytes)", len(b))
	}
	s := Sentence{
		Text:           b,
		Received:       received,
		identifier:     [5]byte{b[1], b[2], b[3], b[4], b[5]},
		parts:          uint8(b[7] - byte('0')),
		part_index:     uint8(b[9] - byte('1')),
		smid_i:         10,
		smid_b:         byte('*'),
		channel:        byte('*'),
		payloadStart:   255,
		payloadEnd:     0,
		Padding:        255,
		checksumPassed: true,
	}

	empty := 0
	smid := b[11]
	channel := b[13] // A or B, or 1 or 2, or empty
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
	s.Padding = uint8(b[lastComma+1] - byte('0'))
	if s.parts > 9 || s.parts == 0 {
		return s, fmt.Errorf("parts is not a positive digit")
	} else if after == 1 {
		return s, nil // no checksum
	} else if after != 4 {
		return s, fmt.Errorf("error in padding or checksum (len: %d)", after)
	}

	s.checksumPassed = ais.Nmea183ChecksumCheck(string(b[:lastComma+5]))
	// A message with a failed checksum might be used to discard an existing incomplete message.
	return s, nil

	// An untested reimplementation of Nmea183ChecksumCheck:
	// sum := 0
	// for _, b := range b[1:lastComma+1] {
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
	} // last is M for over the air, O from ourself/ownship (kystverket transmits a few of those)
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
	} else if s.part_index >= s.parts { // unly used if parts != 1
		return fmt.Errorf("part is not a digit or too high")
	} else if s.smid_i > 10 { // only used if parts != 1
		return fmt.Errorf("smid is not a digit but %c", s.smid_b)
	} else if s.Padding > 5 { // sometimes 6, only used for messages we want to decode
		return fmt.Errorf("padding is not a digit but %c", byte(s.Padding)+byte('0'))
	} else if s.smid_b == byte('*') && s.parts != 1 { // pretty common
		return fmt.Errorf("multipart message without smid")
	} else if s.smid_b != byte('*') && s.parts == 1 { // pretty common
		return fmt.Errorf("standalone sentence with smid")
	} else if s.smid_b != byte('*') && s.smid_i == 10 {
		return fmt.Errorf("smid is kinda wrong: %c", s.smid_b)
	} else if s.channel != byte('A') && s.channel != byte('B') {
		if s.channel == byte('1') || s.channel == byte('2') {
			s.channel = s.channel - byte('1') + byte('A')
		} else if s.channel != byte('*') {
			return fmt.Errorf("Unrecognized channel: %c", s.channel)
		}
	} else if s.Padding < byte('0') || s.Padding > byte('9') {
		return fmt.Errorf("padding is not a number but %c", s.Padding)
	}
	emptySmid := 0
	if s.smid_b == byte('*') {
		emptySmid = 1
	}
	empty := emptySmid
	if s.channel == byte('*') {
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
				expect, at, s.Text[at], s.channel)
		}
	}
	return nil
}

// Returns a view of the ASCII-armored payload
func (s Sentence) Payload() []byte {
	return s.Text[s.payloadStart:s.payloadEnd]
}

// An incomplete message with a certain smid.
// The smid itself is not stored because it's the kay and this is the value.
// The struct is big, but it'r reused.
type incompleteMessage struct {
	sentences [9]Sentence // longest message takes 4-5 sentences, 9 for future-proofing
	have      uint16      // bit field: if least significant bit N is set, sentence with part_index N is received
	parts     uint8       // != sentences[0] parts because [0] might not have been received
	missing   uint8       // = parts - number of bits set in have
	started   time.Time   // THe time of the first received part is the closest to when it was sent
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
	mm[s.smid_i].started = s.Received
	mm[s.smid_i].have = 1 << s.part_index
	mm[s.smid_i].parts = s.parts
	mm[s.smid_i].missing = s.parts - 1
}

// s should not have failed the checksum
func (mm *multiMerger) accept(s Sentence) ([]Sentence, error) {
	if s.parts < 2 {
		return []Sentence{s}, nil
	} else if s.part_index >= s.parts {
		return nil, fmt.Errorf("part is not a digit or too high")
	} else if s.smid_i > 10 { // 10 is used for empty, this check doesn't catch '9'+1
		return nil, fmt.Errorf("smid is not a digit but %c", s.smid_b)
	} else if mm[s.smid_i].missing == 0 {
		mm.restart_with(s)
		return nil, nil
	} else if mm[s.smid_i].parts != s.parts {
		mm.restart_with(s)
		return nil, fmt.Errorf("Out of order")
	} else if mm[s.smid_i].have&(1<<s.part_index) != 0 {
		mm.restart_with(s)
		return nil, fmt.Errorf("Already got")
	} else if s.Received.Sub(mm[s.smid_i].started) >= MAX_MULTI_TIMESPAN {
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

// Invalidate message if one that failed the checksum has the same smid and part,
// and the part index isn't already received.
func (mm *multiMerger) reject(s Sentence) bool {
	if s.parts < 2 ||
		mm[s.smid_i].missing == 0 ||
		mm[s.smid_i].parts != s.parts ||
		s.Received.Sub(mm[s.smid_i].started) >= MAX_MULTI_TIMESPAN ||
		mm[s.smid_i].have&(1<<s.part_index) != 0 {
		return false
	} else {
		mm.reset(s.smid_i)
		return true
	}
}

// Sends sentences and timestamp from the reader goroutine to a reader-specific backend:
// The idea behind splitting the parsing in two parts was to make it easy to see
// weither the reader is keeping up with the source.
type sendSentence struct {
	received time.Time
	text     []byte
}

// Splits and merges packets into sentences, and merge sentences into messages.
// For sentences that span across packets, the timestamp of the last packet is
// used for simplicity. This is not optimal but they should be close enough for
// it not to matter.
type PacketParser struct {
	incomplete     []byte
	async          chan sendSentence
	maxInChan      uint32
	sentencesSplit uint32 // across packets
	sourceName     string
}

// Parse individual sentences and group multi-sentence messages.
// Returns when pp.async is closed
func (pp *PacketParser) decodeSentences(out chan<- *Message) {
	mm := newMultiMerger()
	ok := 0
	logbad := func(source []byte, why string, args ...interface{}) {
		c := AisLog.Compose(LOG_DEBUG)
		if ok != 0 {
			c.Writeln("...%d ok...", ok)
			ok = 0
		}
		c.Writeln(Escape(source))
		c.Finish(why, args...)
	}
	for sentence := range pp.async {
		s, err := parseSentence(sentence.text, sentence.received)
		// err = s.validateSentence(err)
		if err != nil {
			logbad(sentence.text, err.Error())
			continue
		}
		if !s.checksumPassed {
			dropped := mm.reject(s)
			logbad(s.Text, "Checksum failed, incomplete message dropped: %t", dropped)
			continue
		}
		ok++
		sentences, err := mm.accept(s)
		if err != nil {
			AisLog.Debug("Incomplete message dropped for %s", err.Error())
		} else if len(sentences) != 0 {
			out <- NewMessage(pp.sourceName, sentences)
		}
	}
}

/* The synchronous side */

// Spawns a goroutine with a reference to the returned struct.
// Call .Close() to stop it.
func NewPacketParser(source string, dst chan<- *Message) *PacketParser {
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

// Must not be called in parallell with with Accept()
func (pp *PacketParser) Log(lc LogComposer, _ time.Duration) {
	lc.Finish("\tmax in channel: %d/%d, sentences split: %d",
		pp.maxInChan, cap(pp.async), pp.sentencesSplit)
	pp.sentencesSplit = 0
	pp.maxInChan = 0
}

// Merge and split packets into sentences,
// and send the copied sentences to a channel.
// bufferSlice cannot be sent to buffered channels because slicing doesn't copy.
// Might block on said channel.
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
			pp.sentencesSplit++
			return
		}
		pp.send(s, received)
	}
}

// Log if the channel is full, and then send (possibly blocking)
func (pp *PacketParser) send(sentence []byte, received time.Time) {
	current := uint32(len(pp.async))
	if current > pp.maxInChan {
		pp.maxInChan = current
	}
	pp.async <- sendSentence{
		received: received,
		text:     sentence,
	}
}

// Get the first sentence from a packet buffer.
// Returns (copiedSentence, incomplete, remaining)
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

	if nextm1 == -1 && end == -1 { // incomplete sentence
		// cpy = copy but not a builtin
		// slices are hard, make sure we actually create a copy
		cpy := make([]byte, 0, len(bufferSlice))
		cpy = append(cpy, bufferSlice...)
		return cpy, true, []byte{}
	} else if end == -1 || (nextm1 != -1 && nextm1+1 < end) { // no newline before next sentence
		cpy := make([]byte, 0, nextm1+1)
		cpy = append(cpy, bufferSlice[:nextm1+1]...)
		cpy = append(cpy, byte('\r'), byte('\n'))
		return cpy, false, bufferSlice[nextm1+1:]
	} else if end == 0 || bufferSlice[end-1] != byte('\r') { // only \n
		// ECC uses \n, kystverket uses \r\n, normalize to \r\n for forwarding
		cpy := make([]byte, 0, end+2)
		cpy = append(cpy, bufferSlice[:end]...)
		cpy = append(cpy, byte('\r'), byte('\n'))
		return cpy, false, bufferSlice[end+1:]
	} else { // Both \r and \n
		cpy := make([]byte, 0, end+1)
		cpy = append(cpy, bufferSlice[:end+1]...)
		return cpy, false, bufferSlice[end+1:]
	}
}
