package nmeais

import (
	"testing"

	l "github.com/tormol/AIS/logger"
)

var testPackets = []struct {
	incomplete string
	packet     string
	sentence   string
	used       int
}{
	{"", "", "", -1},
	{"", "!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F\r\n", "!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F\r\n", 49},
	{"", "!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F\n", "!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F\r\n", 48},
	{"", "!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F!", "!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F\r\n", 47},
	{"", "!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F", "!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F", -1},
	{"", "noise!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F!", "!BSVDM,1,1,,A,14S:Eb001ePRmHBTAAFnrmV60PRk,0*1F\r\n", 47},
	{"!", "BSVDM,2,2,7,B,00000000000,2*39\r\n", "!BSVDM,2,2,7,B,00000000000,2*39\r\n", 32},
	{"", "BSVDM,2,2,7,B,00000000000,2*39\r\n", "", -1},
	{"!BSVDM,1,1,,A,33nE", "!BSVDM,1,1,,B,144atH00000Lf9nSffVf49TP00S9,0*1D\r\n", "!BSVDM,1,1,,A,33nE\r\n", 0},
	{"!BSVDM,2,2,8,B,88888888880,2*36", "\r\n!BSVD", "!BSVDM,2,2,8,B,88888888880,2*36\r\n", 2},
	{"!BSVDM,2", ",2,,,2CQSp888880,2*0F\n!next", "!BSVDM,2,2,,,2CQSp888880,2*0F\r\n", 22},
	{"!AIVDM,2,2,,,00", "", "!AIVDM,2,2,,,00", -1},
	{"!AIVDM,1,1,,B,ENk`so91S@@@@@@@@@@@@@@@@@@==Fm;9bGh000003vP000,2*11\r", "\n", "!AIVDM,1,1,,B,ENk`so91S@@@@@@@@@@@@@@@@@@==Fm;9bGh000003vP000,2*11\r\n", 1},
}

func TestPackets(t *testing.T) {
	for i, test := range testPackets {
		s, used := FirstSentenceInBuffer([]byte(test.incomplete), []byte(test.packet))
		if string(s) != test.sentence || used != test.used {
			t.Errorf("test %d:\n(\"%s\", \"%s\") ->\n(%d, \"%s\"): got\n(%d, \"%s\")", i,
				l.Escape([]byte(test.incomplete)), l.Escape([]byte(test.packet)),
				test.used, l.Escape([]byte(test.sentence)),
				used, l.Escape(s))
		}
	}
}
