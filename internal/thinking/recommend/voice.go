package recommend

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/voice"
)

// announceTopN bounds the spoken recs per spec §7 voice mode — the morning
// brief renders up to 3 and the voice surface mirrors that ceiling so
// operator memory between channels stays aligned.
const announceTopN = 3

// announceScope is the contracts.Attestor scope. Privileged because the
// daemon audibly leaks pattern names (which encode what surfaces have
// learned from the operator) through the room's speaker.
const announceScope = "recommend:announce"

// VoiceAnnouncer speaks the top recommendations through the TTS chain on
// operator pull ("what should I do") and — once the focus-block hook lands
// — on idle push. The announce path is gated by contracts.Attestor so a
// stolen mic cannot exfiltrate learned patterns through the speaker.
type VoiceAnnouncer struct {
	tts    voice.TTS
	attest contracts.Attestor
}

// NewVoiceAnnouncer wires a TTS chain and an Attestor. tts is typically
// *voice.ChainTTS but any voice.TTS works; the concrete chain is selected
// at process boot by voice.NewTTS.
func NewVoiceAnnouncer(tts voice.TTS, attest contracts.Attestor) *VoiceAnnouncer {
	return &VoiceAnnouncer{tts: tts, attest: attest}
}

// Announce speaks the top-N ranked recs as a single utterance. Behavior:
//   - empty input → graceful skip, no attestation consumed (no privileged
//     side effect occurs, so the consent prompt would just be noise);
//   - blocked/silent tiers are filtered before ranking — those tiers are
//     defined to never reach the operator surface;
//   - recs are ranked by Confidence descending; ties broken by ID for
//     deterministic ordering across runs.
func (a *VoiceAnnouncer) Announce(ctx context.Context, recs []Recommendation) error {
	speakable := filterSpeakable(recs)
	if len(speakable) == 0 {
		return nil
	}
	if err := a.attest.Attest(ctx, announceScope); err != nil {
		return fmt.Errorf("recommend/voice: attest: %w", err)
	}
	sort.SliceStable(speakable, func(i, j int) bool {
		if speakable[i].Confidence != speakable[j].Confidence {
			return speakable[i].Confidence > speakable[j].Confidence
		}
		return speakable[i].ID < speakable[j].ID
	})
	if len(speakable) > announceTopN {
		speakable = speakable[:announceTopN]
	}
	return a.tts.Speak(ctx, formatUtterance(speakable))
}

// filterSpeakable drops tiers that never reach the operator surface.
func filterSpeakable(recs []Recommendation) []Recommendation {
	out := make([]Recommendation, 0, len(recs))
	for _, r := range recs {
		if r.Tier == TierAuto || r.Tier == TierConfirm {
			out = append(out, r)
		}
	}
	return out
}

// formatUtterance renders one human utterance. Confidence is quantized to
// whole percent — TTS prosody on "fifty-five point three percent" reads as
// false precision when the underlying signal is α-blended counts.
func formatUtterance(recs []Recommendation) string {
	var b strings.Builder
	b.WriteString("Top recommendations: ")
	for i, r := range recs {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s at %d percent", r.Pattern, int(r.Confidence*100+0.5))
	}
	b.WriteString(".")
	return b.String()
}
