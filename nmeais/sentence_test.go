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
