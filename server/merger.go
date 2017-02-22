package main

import (
	ais "github.com/andmarios/aislib"
	"time"
)

type Message struct {
	Source    string     // AIS listener
	Sentences []Sentence // one or more AIS sentences
	Received  time.Time  // of last received sentence
	Type      uint8
}

func dearmorByte(b byte) uint8 {
	v := uint8(b) - 48
	if v > 40 {
		v -= 8
	}
	// TODO validation ?
	return v & 0x3f // 0b0011_1111
}
func NewMessage(sourceName string, sentences []Sentence) *Message {
	return &Message{
		Received:  sentences[0].Received,
		Sentences: sentences,
		Source:    sourceName,
		Type:      dearmorByte(sentences[0].Payload()[0]),
	}
}
func (m *Message) dearmoredPayload() []uint8 {
	// Completely untested
	data := make([]uint8, 0, len(m.Sentences[0].Payload())*8/6)
	bitbuf := uint32(0)
	bits := uint(0)
	for i := range m.Sentences {
		for _, b := range m.Sentences[i].Payload() {
			bitbuf = (bitbuf << 6) | uint32(dearmorByte(b))
			bits += 6
			if bits >= 24 { // leave one byte in case it's the last and there is padding
				bits -= 8
				data = append(data, uint8(bitbuf>>bits))
				bits -= 8
				data = append(data, uint8(bitbuf>>bits))
			}
		}
		pad := uint(m.Sentences[i].Padding)
		// TODO validated pad and handle 6
		// if pad > 6 {// FIXME report error and discard message
		// 	return fmt.Errorf("padding is not a digit but %c", byte(s.Padding)+byte('0'))
		// }
		pad = 5 - pad // I REALLY doubt this is correct, but esr says so..
		bits -= pad
		bitbuf >>= pad
	}
	for bits >= 8 {
		bits -= 8
		data = append(data, uint8(bitbuf>>bits))
	}
	return data
}
func (m *Message) armoredPayload() string {
	if len(m.Sentences) == 1 {
		return string(m.Sentences[0].Payload())
	} else {
		combined := make([]byte, 0, 2*len(m.Sentences[0].Payload()))
		for i := range m.Sentences {
			combined = append(combined, m.Sentences[i].Payload()...)
		}
		return string(combined)
	}
}
func (m *Message) UnescapedText() string {
	if len(m.Sentences) == 1 {
		return string(m.Sentences[0].Text)
	} else {
		combined := make([]byte, 0, 2*len(m.Sentences[0].Text))
		for i := range m.Sentences {
			combined = append(combined, m.Sentences[i].Text...)
		}
		return string(combined)
	}
}

type mergeStats struct {
	total      uint
	duplicates uint
}

func Merge(in <-chan *Message, forward chan<- *Message) {
	dt := NewDuplicateTester(1000 * time.Millisecond)
	for msg := range in {
		if dt.IsRepeated(msg) {
			continue
		}
		forward <- msg
		// TODO register
		// TODO stats
		mmsi := uint32(0)
		var err error
		ps := (*ais.PositionReport)(nil)
		switch msg.Type {
		case 1, 2, 3: // class A position report (longest)
			cpar, e := ais.DecodeClassAPositionReport(msg.armoredPayload())
			ps = &cpar.PositionReport
			mmsi, err = ps.MMSI, e
		case 18: // basic class B position report (shorter)
			cpbr, e := ais.DecodeClassBPositionReport(msg.armoredPayload())
			ps = &cpbr.PositionReport
			mmsi, err = ps.MMSI, e
		case 19: // extended class B position report (longer)
		case 27: // long-range broadcast (shortest)
		case 5: // static voiage data
			svd, e := ais.DecodeStaticVoyageData(msg.armoredPayload())
			mmsi, err = svd.MMSI, e
		case 24: // static data
		// case 11: // whishlist UTC/Date response
		// 	// fallthrough // identical to 4, but aislib doesn't accept it
		// case 4: // whishlist base station, might improve timestamps
		// 	bsr, e := ais.DecodeBaseStationReport(msg.armoredPayload())
		// 	mmsi, err = bsr.MMSI, e
		// case 21: // whishlist aid-to-navigation report, could be shown on maps
		default:
			// AisLog.Info("other type: %d", msg.Type) // whishlist log better
		}
		if mmsi != 0 {
			//		AisLog.Info("%09d: %02d", mmsi, msg.Type)
		}
		if err != nil {
			AisLog.Debug("Bad payload of type %d from %d: %s",
				msg.Type, mmsi, err.Error())
		}
	}
}
