package main

import (
	"sync"
	"time"

	"github.com/tormol/AIS/logger"
	"github.com/tormol/AIS/nmeais"
)

const (
	maxSentencesBetween = 7
	maxMessageTimespan  = 3 * time.Second
)

// PacketParser splits and merges packets into sentences, and merges sentences into messages.
// For sentences that span across packets, the timestamp of the last packet is
// used for simplicity. This is not optimal but they should be close enough for it not to matter.
type PacketParser struct {
	incomplete []byte
	async      chan sendSentence // stored to let Close() close it
	SourceName string
	logger     *logger.Logger
	pl         packetLogger
}

// NewPacketParser creates a new PacketParser
// Spawns a goroutine with a reference to the returned struct.
// Call .Close() to stop it.
func NewPacketParser(source string, log *logger.Logger, dst func(*nmeais.Message)) *PacketParser {
	pp := &PacketParser{
		async:      make(chan sendSentence, 200),
		SourceName: source,
		logger:     log,
		pl:         newPacketLogger(),
	}
	Log.AddPeriodicLogger(pp.SourceName+"_packets", 40*time.Second,
		func(l *logger.Logger, s time.Duration) {
			c := log.Compose(logger.LOG_DEBUG)
			c.Writeln("%s", pp.SourceName)
			pp.pl.log(c, s)
		},
	)
	go decodeSentences(pp, dst)
	return pp
}

// Close stops the internal goroutine and removes the periodic logger.
func (pp *PacketParser) Close() {
	close(pp.async)
	Log.RemovePeriodicLogger(pp.SourceName + "_packets")
}

// Accept merges and splits packets into sentences,
// and then sends the copied sentence(s) to a channel.
// Will block on that channel if it is full.
// (bufferSlice cannot be sent to buffered channels because slicing doesn't copy.)
func (pp *PacketParser) Accept(bufferSlice []byte, received time.Time) {
	if len(pp.incomplete) == 0 && len(bufferSlice) != 0 && bufferSlice[0] != byte('!') {
		pp.logger.Debug("%s\nPacket doesn't start with '!'", logger.Escape(bufferSlice))
	}
	pp.pl.register(len(pp.incomplete) != 0, bufferSlice, received)
	for len(bufferSlice) != 0 {
		sText, used := nmeais.FirstSentenceInBuffer(pp.incomplete, bufferSlice)
		if used == -1 {
			pp.incomplete = sText
			return
		}
		pp.incomplete = []byte{}
		if len(sText) == 0 && len(bufferSlice) == used {
			pp.logger.Debug("%s\nNo sentence in packet", logger.Escape(bufferSlice))
			return
		}
		bufferSlice = bufferSlice[used:]
		pp.async <- sendSentence{
			received: received,
			text:     sText,
		}
	}
}

// Sends sentences and timestamp from the reader goroutine to a reader-specific backend:
// The idea behind splitting the parsing in two parts was to make it easy to see
// weither the reader is keeping up with the source.
type sendSentence struct {
	received time.Time
	text     []byte
}

// Parse individual sentences and group multi-sentence messages.
// Returns when pp.async is closed.
// Is ran in a goroutine started by NewPacketParser.
func decodeSentences(pp *PacketParser, callback func(*nmeais.Message)) {
	ma := nmeais.NewMessageAssembler(maxSentencesBetween, maxMessageTimespan, pp.SourceName)
	ok := 0
	logbad := func(source []byte, why string, args ...interface{}) {
		c := pp.logger.Compose(logger.LOG_DEBUG)
		if ok != 0 {
			c.Writeln("%s: ...%d ok...", pp.SourceName, ok)
			ok = 0
		}
		c.Writeln(logger.Escape(source))
		c.Finish(why, args...)
	}
	for sentence := range pp.async {
		s, err := nmeais.ParseSentence(sentence.text, sentence.received)
		// err = s.Validate(err)
		if err != nil {
			logbad(sentence.text, err.Error())
			continue
		}
		ok++
		message, err := ma.Accept(s)
		if err != nil {
			logbad(sentence.text, "Incomplete message dropped: %s", err.Error())
		}
		if message != nil {
			callback(message)
		}
	}
}

// PacketHandler collects statistics, logs it and forwards the packets to PacketParser.
type packetLogger struct {
	started             time.Time
	statsLock           sync.Mutex // Simpler and possibly even faster than atomic operations for n fields
	readTime            time.Duration
	packets             uint64
	splitSentences      uint64 // across packets
	bytes               uint64
	totalReadTime       time.Duration
	totalSplitSentences uint64
	totalBytes          uint64
	totalPackets        uint64
}

func newPacketLogger() packetLogger {
	return packetLogger{
		started: time.Now(),
	}
}

// Log prints some statistics to lc.
// It must not be called in parallell with with Accept().
func (pl *packetLogger) log(lc logger.LogComposer, sinceLast time.Duration) {
	pl.statsLock.Lock()
	defer pl.statsLock.Unlock()

	pl.totalBytes += pl.bytes
	pl.totalPackets += pl.packets
	pl.totalReadTime += pl.readTime
	pl.totalSplitSentences += pl.splitSentences
	avg := time.Duration(0)
	if pl.packets != 0 {
		avg = time.Duration(pl.readTime.Nanoseconds()/int64(pl.packets)) * time.Nanosecond
	}
	totalAvg := time.Duration(0)
	if pl.totalPackets != 0 {
		totalAvg = time.Duration(pl.totalReadTime.Nanoseconds()/int64(pl.totalPackets)) * time.Nanosecond
	}

	now := time.Now()
	lc.Writeln("\ttotal: listened for %s/%s, %sB, %s/%s packets w/split sentence, avg read: %s",
		logger.RoundDuration(pl.totalReadTime, time.Second),
		logger.RoundDuration(now.Sub(pl.started), time.Second),
		logger.SiMultiple(pl.totalBytes, 1024, 'M'),
		logger.SiMultiple(pl.totalSplitSentences, 1000, 'M'),
		logger.SiMultiple(pl.totalPackets, 1000, 'M'),
		totalAvg.String(),
	)
	lc.Writeln("\tsince last: %s/%s, %sB, %s/%s packets w/split sentence, avg read: %s",
		logger.RoundDuration(pl.readTime, time.Second),
		logger.RoundDuration(sinceLast, time.Second),
		logger.SiMultiple(pl.bytes, 1024, 'M'),
		logger.SiMultiple(pl.splitSentences, 1000, 'M'),
		logger.SiMultiple(pl.packets, 1000, 'M'),
		avg.String(),
	)
	lc.Close()

	pl.splitSentences = 0
	pl.bytes = 0
	pl.packets = 0
	pl.readTime = 0
}

func (pl *packetLogger) register(incomplete bool, bufferSlice []byte, readStarted time.Time) {
	now := time.Now()
	pl.statsLock.Lock()
	pl.readTime += now.Sub(readStarted)
	pl.packets++
	pl.bytes += uint64(len(bufferSlice))
	if incomplete {
		pl.splitSentences++
	}
	pl.statsLock.Unlock()
}
