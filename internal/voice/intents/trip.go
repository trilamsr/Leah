package intents

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// IntentKind enumerates the trip-planning utterances this package
// recognises. Stable across the MVP so the dispatcher's switch is a
// compile-time-checked surface.
type IntentKind int

const (
	IntentUnknown IntentKind = iota
	IntentOnTheWay
	IntentMeetInMiddle
	IntentSuggestTrip
)

// TripIntent is the recognised utterance envelope. Origin is empty for
// utterances rooted at the operator's current location — Dispatch
// substitutes the operator's known origin (or "current location" sent
// straight to the planner adapter, which resolves it).
type TripIntent struct {
	Kind       IntentKind
	Origin     string
	Dest       string
	Categories []string
}

// Place mirrors tripplanner.Place's spoken-relevant fields. Declared
// locally so this package has no compile-time edge to tripplanner.
type Place struct {
	Name       string
	Categories []string
	Rating     float32
}

// CorridorOpts mirrors tripplanner.CorridorOpts. Same rationale: keep the
// seam structural so adapter changes don't cascade into voice/intents.
type CorridorOpts struct {
	MaxDetourMinutes int
	Categories       []string
	OpenAt           time.Time
	SampleEveryM     int
	TopK             int
	RadiusM          int
}

// TripPlannerSeam is the planner surface this package depends on.
// tripplanner.Composer satisfies it through a one-line adapter; tests
// bind a fake.
type TripPlannerSeam interface {
	OnTheWay(ctx context.Context, origin, dest string, opts CorridorOpts) ([]Place, error)
	MeetInMiddle(ctx context.Context, partyA, partyB string, category string) ([]Place, error)
}

// TTSSeam is the speak surface this package depends on. voice.ChainTTS
// satisfies it directly.
type TTSSeam interface {
	Speak(ctx context.Context, text string) error
}

// Router recognises a transcript and emits a TripIntent. Returns ok=false
// (and a zero TripIntent) when the transcript is not a trip utterance —
// session arbitration falls through to the general reasoner.
type Router interface {
	Recognize(ctx context.Context, transcript string) (TripIntent, bool, error)
}

// announceTopK is the default number of results the handler speaks. 3
// fits the operator's spoken attention budget; tunable via Handler.TopK.
const announceTopK = 3

// reOnTheWay matches "what's [on|along] my drive to <dest>" and
// "what's nearby on my drive to <dest>". Captures the destination as
// group 1.
var reOnTheWay = regexp.MustCompile(`(?i)what'?s\s+(?:nearby\s+)?(?:on|along)\s+my\s+drive\s+to\s+(.+?)[?.!]?$`)

// reMeetInMiddle matches "find <category> [midway|halfway] between me
// and <name>". Captures category as group 1 and the other party as
// group 2.
var reMeetInMiddle = regexp.MustCompile(`(?i)find\s+(\w+)\s+(?:midway|halfway)\s+between\s+(?:me|us)\s+and\s+(.+?)[?.!]?$`)

// RegexRouter is the MVP recognition implementation — regex + keyword
// matching, no LLM call. Stateless; safe to share across goroutines.
type RegexRouter struct{}

// NewRegexRouter returns a ready-to-use router. The constructor exists
// so test wiring stays uniform with the Handler constructor pattern.
func NewRegexRouter() *RegexRouter { return &RegexRouter{} }

// Recognize parses transcript into a TripIntent. The error return is
// reserved for future LLM-backed routers that may have transport
// failures; the regex implementation never returns a non-nil error.
func (*RegexRouter) Recognize(_ context.Context, transcript string) (TripIntent, bool, error) {
	t := strings.TrimSpace(transcript)
	if t == "" {
		return TripIntent{}, false, nil
	}
	if m := reOnTheWay.FindStringSubmatch(t); m != nil {
		return TripIntent{
			Kind: IntentOnTheWay,
			Dest: strings.TrimSpace(m[1]),
		}, true, nil
	}
	if m := reMeetInMiddle.FindStringSubmatch(t); m != nil {
		return TripIntent{
			Kind:       IntentMeetInMiddle,
			Origin:     "me",
			Dest:       strings.TrimSpace(m[2]),
			Categories: []string{strings.ToLower(strings.TrimSpace(m[1]))},
		}, true, nil
	}
	return TripIntent{}, false, nil
}

// Handler dispatches recognised intents to the planner and announces the
// top-K results through the TTS seam. Both seams are required; nil
// values cause Dispatch to return an error rather than panic three
// layers deep.
type Handler struct {
	Planner   TripPlannerSeam
	Announcer TTSSeam
	// TopK overrides the announceTopK default. Zero adopts the default.
	TopK int
	// DefaultOrigin is substituted when the recognised intent leaves
	// Origin empty (operator utterances rooted at "my drive" don't
	// name the start point). Empty default forwards the planner's
	// adapter-level "current location" handling.
	DefaultOrigin string
}

// Dispatch routes the intent and speaks the result. Planner errors are
// announced to the operator (so a silent failure doesn't masquerade as
// "no results") and returned for upstream logging.
func (h *Handler) Dispatch(ctx context.Context, intent TripIntent) error {
	if h.Planner == nil {
		return errors.New("voice/intents: Handler.Planner required")
	}
	if h.Announcer == nil {
		return errors.New("voice/intents: Handler.Announcer required")
	}
	switch intent.Kind {
	case IntentOnTheWay:
		return h.dispatchOnTheWay(ctx, intent)
	case IntentMeetInMiddle:
		return h.dispatchMeetInMiddle(ctx, intent)
	default:
		return fmt.Errorf("voice/intents: unhandled intent kind %d", intent.Kind)
	}
}

func (h *Handler) dispatchOnTheWay(ctx context.Context, intent TripIntent) error {
	origin := intent.Origin
	if origin == "" {
		origin = h.DefaultOrigin
	}
	places, err := h.Planner.OnTheWay(ctx, origin, intent.Dest, CorridorOpts{
		Categories: intent.Categories,
		TopK:       h.topK(),
	})
	if err != nil {
		_ = h.Announcer.Speak(ctx, fmt.Sprintf("Trip planner error: %s", err.Error()))
		return err
	}
	return h.Announcer.Speak(ctx, renderPlaces(intent.Dest, places, h.topK()))
}

func (h *Handler) dispatchMeetInMiddle(ctx context.Context, intent TripIntent) error {
	category := ""
	if len(intent.Categories) > 0 {
		category = intent.Categories[0]
	}
	partyA := intent.Origin
	if partyA == "" {
		partyA = h.DefaultOrigin
	}
	places, err := h.Planner.MeetInMiddle(ctx, partyA, intent.Dest, category)
	if err != nil {
		_ = h.Announcer.Speak(ctx, fmt.Sprintf("Trip planner error: %s", err.Error()))
		return err
	}
	return h.Announcer.Speak(ctx, renderMidpointPlaces(intent.Dest, category, places, h.topK()))
}

func (h *Handler) topK() int {
	if h.TopK > 0 {
		return h.TopK
	}
	return announceTopK
}

// renderPlaces builds the spoken summary for an OnTheWay dispatch. Names
// only — never coordinates — per the brief's no-lat-lng-spoken rule.
func renderPlaces(dest string, places []Place, k int) string {
	if len(places) == 0 {
		return fmt.Sprintf("Nothing notable on the drive to %s.", dest)
	}
	if k > len(places) {
		k = len(places)
	}
	names := make([]string, k)
	for i := 0; i < k; i++ {
		names[i] = places[i].Name
	}
	return fmt.Sprintf("On the drive to %s: %s.", dest, strings.Join(names, ", "))
}

func renderMidpointPlaces(other, category string, places []Place, k int) string {
	if len(places) == 0 {
		return fmt.Sprintf("No %s found midway between you and %s.", category, other)
	}
	if k > len(places) {
		k = len(places)
	}
	names := make([]string, k)
	for i := 0; i < k; i++ {
		names[i] = places[i].Name
	}
	return fmt.Sprintf("Midway between you and %s: %s.", other, strings.Join(names, ", "))
}
