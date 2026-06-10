// Package facetime is Leah's adapter for initiating FaceTime calls from the
// host Mac via macOS URL schemes (facetime://, facetime-audio://). It exposes
// InitiateVideo / InitiateAudio, routes every launch through the operator-
// attestation gate before invoking open(1), and inlines a hardcoded rolling
// rate limit as a robo-dial mitigation. Apple's pre-call UI confirmation is
// the human-in-the-loop that prevents fully-unattended outbound calls; this
// adapter is the seam, not a workaround.
package facetime
