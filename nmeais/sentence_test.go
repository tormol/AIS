package nmeais

import (
	"fmt"
	"io/ioutil"
	"log"
	"testing"

	l "github.com/tormol/AIS/logger"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

var testBadChecksumHex = []string{
	"99",
	"3a",
	string([]byte{0, 0}),
	string([]byte{'0', '9' + 1}),
	string([]byte{'0' - 1, '8'}),
	string([]byte{'A' - 1, 'A'}),
	string([]byte{'6', 'G'}),
}
var testChecksumText = []struct {
	text     string
	checksum byte
	result   ChecksumResult
}{
	{"", 0, ChecksumPassed},
	{string([]byte{0x00}), 0x00, ChecksumPassed},
	{string([]byte{0x00}), 0x01, ChecksumFailed},
	{string([]byte{0x00}), 0x10, ChecksumFailed},
	{string([]byte{0x7f}), 0x7f, ChecksumPassed},
	{string([]byte{0x7f}), 0xff, ChecksumFailed},
	{string([]byte{0xff}), 0xff, ChecksumFailed},
	{string([]byte{0x00}), 0x80, ChecksumFailed},
	{"AA", 0, ChecksumPassed},
	{"aa", 0, ChecksumPassed},
	{"aaa", 'a', ChecksumPassed},
	{"aaaa", 0, ChecksumPassed},
	{"abcd", 'a' ^ 'b' ^ 'c' ^ 'd', ChecksumPassed},
	{"abcd", 'd' ^ 'c' ^ 'b' ^ 'a', ChecksumPassed},
	{"abcd", 'e' ^ 'd' ^ 'c' ^ 'b', ChecksumFailed},
	{"bcde", 'a' ^ 'b' ^ 'c' ^ 'd', ChecksumFailed},
	{"BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0", 0x1f, ChecksumPassed},
	{"BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0", 0x0f, ChecksumFailed},
	{"BSVDM,1,1,,A,13nMoF00000H56fQwFDLFD<800Rg,0", 0x71, ChecksumPassed},
	{"BSVDM,1,1,,A,13nMoF00000H56fQwFDLFD<800Rg,0", 0x17, ChecksumFailed},
	{"BSVDM,1,1,,B,144atH00000Lf9nSffVf49TP00S9,0", 0x1D, ChecksumPassed},
}

func TstCheckChecksum(t *testing.T) {
	for _, badHexStr := range testBadChecksumHex {
		// need to test every possible checksum, because I don't know what
		// checksum will be returned if there is a bug.
		for i := 0; i < 256; i++ {
			text := string(byte(i))
			result := checkChecksum([]byte(text), badHexStr[0], badHexStr[1])
			if result != ChecksumFailed {
				t.Errorf("\"%s\" should not be a valid checksum\n", badHexStr)
				break // to next test case
			}
		}
	}
	half2hex := func(half byte) byte {
		half &= 0x0f
		if half > 9 {
			return 'A' + half - 10
		}
		return '0' + half
	}
	for i, test := range testChecksumText {
		first := half2hex(test.checksum >> 4)
		second := half2hex(test.checksum & 0x0f)
		result := checkChecksum([]byte(test.text), first, second)
		if result != test.result {
			r := "passed"
			if result == ChecksumPassed {
				r = "failed"
			}
			t.Errorf("%2d: checkChecksum(\"%s\", 0x%x) (len %d) should have %s",
				i, test.text, test.checksum, len(test.text), r,
			)
		}
	}
}

var testSentences = []struct {
	text        string
	parseErr    string
	validateErr string
	sentence    Sentence
}{
	{"", "too short (0 bytes)", "", Sentence{}},
	{"!ABVDM,1,1,,,,0\r\n", "", "", Sentence{ // shortest possible
		Identifier:   [5]byte{'A', 'B', 'V', 'D', 'M'},
		Parts:        1,
		PartIndex:    0,
		HasSMID:      false,
		SMID:         10,
		Channel:      '*',
		payloadStart: 13,
		payloadEnd:   13,
		padding:      0,
		Checksum:     ChecksumAbsent,
	}},
	{"!ADVDO,9,9,,A,,5\r\n", "", "multipart message without SMID", Sentence{ // shortest possible
		Identifier:   [5]byte{'A', 'D', 'V', 'D', 'O'},
		Parts:        9,
		PartIndex:    8,
		HasSMID:      false,
		SMID:         10,
		Channel:      'A',
		payloadStart: 14,
		payloadEnd:   14,
		padding:      5,
		Checksum:     ChecksumAbsent,
	}},
	{"!AIVDM,2,1,0,,,5\r\n", "", "", Sentence{ // shortest possible
		Identifier:   [5]byte{'A', 'I', 'V', 'D', 'M'},
		Parts:        2,
		PartIndex:    0,
		HasSMID:      true,
		SMID:         0,
		Channel:      '*',
		payloadStart: 14,
		payloadEnd:   14,
		padding:      5,
		Checksum:     ChecksumAbsent,
	}},
	{"!ANVDO,2,2,9,B,,5\r\n", "", "", Sentence{ // shortest possible
		Identifier:   [5]byte{'A', 'N', 'V', 'D', 'O'},
		Parts:        2,
		PartIndex:    1,
		HasSMID:      true,
		SMID:         9,
		Channel:      'B',
		payloadStart: 15,
		payloadEnd:   15,
		padding:      5,
		Checksum:     ChecksumAbsent,
	}},
	{"!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F\r\n", "", "", Sentence{
		Identifier:   [5]byte{'B', 'S', 'V', 'D', 'M'},
		Parts:        1,
		PartIndex:    0,
		HasSMID:      false,
		SMID:         10,
		Channel:      'A',
		payloadStart: 14,
		payloadEnd:   42,
		padding:      0,
		Checksum:     ChecksumPassed,
	}},
	{"!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1E\r\n", "", "", Sentence{
		Identifier:   [5]byte{'B', 'S', 'V', 'D', 'M'},
		Parts:        1,
		PartIndex:    0,
		HasSMID:      false,
		SMID:         10,
		Channel:      'A',
		payloadStart: 14,
		payloadEnd:   42,
		padding:      0,
		Checksum:     ChecksumFailed,
	}},
	{"!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0\r\n", "", "", Sentence{
		Identifier:   [5]byte{'B', 'S', 'V', 'D', 'M'},
		Parts:        1,
		PartIndex:    0,
		HasSMID:      false,
		SMID:         10,
		Channel:      'A',
		payloadStart: 14,
		payloadEnd:   42,
		padding:      0,
		Checksum:     ChecksumAbsent,
	}},
	{"!BSVDM,1,1,9,0,144atH00000Lf9nSffVf49TP00S9,1*00\r\n", "", "standalone sentence with SMID", Sentence{
		Identifier:   [5]byte{'B', 'S', 'V', 'D', 'M'},
		Parts:        1,
		PartIndex:    0,
		HasSMID:      true,
		SMID:         9,
		Channel:      '0',
		payloadStart: 15,
		payloadEnd:   43,
		padding:      1,
		Checksum:     ChecksumFailed,
	}},
	{"$GPAAM,A,A,0,N,WPTNME,5*04\r\n", "", "unrecognized identifier: GPAAM", Sentence{
		Identifier:   [5]byte{'G', 'P', 'A', 'A', 'M'},
		Parts:        'A' - '0',
		PartIndex:    'A' - '1',
		HasSMID:      true,
		SMID:         0,
		Channel:      'N',
		payloadStart: 15,
		payloadEnd:   21,
		padding:      5,
		Checksum:     ChecksumPassed,
	}},
	{"!AIVDM,1,1,,2,456789012345678901234567890,", "error in padding or checksum (-2 characters after payload)", "", Sentence{}},
	{"!12345,2,2,8,0,567890123456789longest7valid3sentence23456789012345678901234,0*77\r\n", "", "unrecognized identifier: 12345", Sentence{
		Identifier:   [5]byte{'1', '2', '3', '4', '5'},
		Parts:        2,
		PartIndex:    1,
		HasSMID:      true,
		SMID:         8,
		Channel:      '0',
		payloadStart: 15,
		payloadEnd:   75,
		padding:      0,
		Checksum:     ChecksumFailed,
	}},
}

func TestSentences(t *testing.T) {
	s2str := func(s Sentence) string {
		return fmt.Sprintf("{\"%s\", %d\\%d, %t:%d, '%c', %d..%d-%d, '%c'}",
			string(s.Identifier[:]), s.Parts, s.PartIndex, s.HasSMID, s.SMID,
			s.Channel, s.payloadStart, s.payloadEnd, s.padding, s.Checksum)
	}
	for i, test := range testSentences {
		test.sentence.Text = test.text
		s, parseErr := ParseSentence([]byte(test.text), test.sentence.Received)
		validateErr := s.Validate(parseErr)
		if test.parseErr != "" {
			if parseErr == nil {
				t.Errorf("%2d: \"%s\"\n   Got value %s\nWanted parse error \"%s\"",
					i, l.Escape([]byte(test.text)), s2str(s), test.parseErr)
			} else if parseErr.Error() != test.parseErr {
				t.Errorf("%2d: \"%s\"\n   Got parse error \"%s\"\nWanted parse error \"%s\"",
					i, l.Escape([]byte(test.text)), parseErr.Error(), test.parseErr)
			}
		} else if parseErr != nil {
			t.Errorf("%2d: \"%s\"\n   Got parse error \"%s\"\nWanted value %s",
				i, l.Escape([]byte(test.text)), parseErr.Error(), s2str(test.sentence))
		} else if s != test.sentence {
			t.Errorf("%2d: \"%s\"\n   Got value %s\nWanted value %s",
				i, l.Escape([]byte(test.text)), s2str(s), s2str(test.sentence))
			if s.Checksum == ChecksumFailed && test.sentence.Checksum == ChecksumPassed {
				checksum := uint8(0)
				for _, b := range []byte(test.text[1 : len(test.text)-5]) {
					checksum ^= b
				}
				t.Logf("(Cheksum: %02X)", checksum)
			}
		} else if test.validateErr != "" {
			if validateErr == nil {
				t.Errorf("%2d: \"%s\"\n   Got value %s\nWanted validate error \"%s\"",
					i, l.Escape([]byte(test.text)), s2str(s), test.validateErr)
			} else if validateErr.Error() != test.validateErr {
				t.Errorf("%2d: \"%s\"\n   Got validate error \"%s\"\nWanted validate error \"%s\"",
					i, l.Escape([]byte(test.text)), validateErr.Error(), test.validateErr)
			}
		} else if validateErr != nil {
			t.Errorf("%2d: \"%s\"\n   Got validate error \"%s\"\nWanted value %s",
				i, l.Escape([]byte(test.text)), validateErr.Error(), s2str(test.sentence))
		}
	}
}
