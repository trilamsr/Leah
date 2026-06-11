// Package intents recognises trip-planning utterances and dispatches them
// to a planner seam, announcing results through a TTS seam. The package
// owns no concrete tripplanner import — both seams (TripPlannerSeam,
// TTSSeam) are declared locally so voice/session can bind doubles in
// tests and the real Composer + ChainTTS in production without inverting
// the layering.
//
// Recognition is regex+keyword for the MVP; LLM-based intent recognition
// is deferred (tracked in Linear, trip-planning milestone).
package intents
