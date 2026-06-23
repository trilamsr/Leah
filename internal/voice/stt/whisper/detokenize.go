package whisper

import (
	_ "embed"
	"encoding/binary"
	"sync"
)

//go:embed assets/whisper_vocab.bin
var whisperVocabBlob []byte

// firstSpecialID gates out Whisper added tokens (language, task, timestamps, sentinels) — they have no visible text.
const firstSpecialID = 50257

var (
	vocabOnce  sync.Once
	vocabTable [firstSpecialID][]byte
)

func loadVocab() {
	const header = 4 + 1 + 4
	b := whisperVocabBlob
	if len(b) < header || string(b[:4]) != "WBPE" {
		return
	}
	count := binary.LittleEndian.Uint32(b[5:9])
	off := header
	for i := uint32(0); i < count; i++ {
		if off+3 > len(b) {
			return
		}
		id := binary.LittleEndian.Uint16(b[off : off+2])
		n := int(b[off+2])
		off += 3
		if off+n > len(b) {
			return
		}
		if int(id) < firstSpecialID {
			vocabTable[id] = b[off : off+n]
		}
		off += n
	}
}

func decodeTokens(ids []int32) string {
	if len(ids) == 0 {
		return ""
	}
	vocabOnce.Do(loadVocab)
	var out []byte
	for _, id := range ids {
		if id < 0 || int(id) >= firstSpecialID {
			continue
		}
		out = append(out, vocabTable[id]...)
	}
	return string(out)
}
