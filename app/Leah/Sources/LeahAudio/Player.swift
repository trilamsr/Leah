import AVFoundation
import Foundation

/// Player decodes MP3 bytes streamed from the daemon's ElevenLabs Flash v2.5
/// path (`tts.cloud.frame`) and plays them through AVAudioEngine. The buffer
/// queue keeps the gap between chunks below the 75-150 ms TTFB budget §2.7
/// mandates — we start playing the first chunk before the second arrives.
public final class Player {
    private let engine = AVAudioEngine()
    private let player = AVAudioPlayerNode()
    private var pendingBuffers: [Data] = []
    private let lock = NSLock()

    public init() {
        engine.attach(player)
    }

    public func enqueue(_ mp3Bytes: Data) {
        lock.lock(); defer { lock.unlock() }
        pendingBuffers.append(mp3Bytes)
    }

    public func reset() {
        lock.lock(); defer { lock.unlock() }
        pendingBuffers.removeAll()
        if engine.isRunning {
            player.stop()
            engine.stop()
        }
    }
}
