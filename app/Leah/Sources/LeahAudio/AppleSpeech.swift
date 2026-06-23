import AVFoundation
import Foundation

/// AppleSpeech wraps AVSpeechSynthesizer at the "Ava (Premium)" voice — the
/// §2.7 offline + privacy-flagged fallback. Daemon-side `tts.apple.speak` IPC
/// frames trigger this directly; no audio bytes traverse IPC on this path.
public final class AppleSpeech {
    private let synth = AVSpeechSynthesizer()
    private let voiceID = "com.apple.voice.premium.en-US.Ava"

    public init() {}

    /// preWarm loads the Ava voice model so the first speak() doesn't pay the
    /// cold-load cost. Called once at app launch.
    public func preWarm() {
        let u = AVSpeechUtterance(string: " ")
        u.voice = AVSpeechSynthesisVoice(identifier: voiceID)
        synth.speak(u)
    }

    public func speak(_ text: String, voice: String) {
        let u = AVSpeechUtterance(string: text)
        u.voice = AVSpeechSynthesisVoice(identifier: voiceID)
        u.rate = 0.5 // matches §2.7 "alto ~145 wpm slow-warm cadence"
        synth.speak(u)
    }
}
