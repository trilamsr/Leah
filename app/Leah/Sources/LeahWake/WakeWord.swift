import AVFoundation
import AppKit
import CoreML
import Foundation

/// WakeWord is the §6.7 opt-in wake-word adapter; armed defaults FALSE — no
/// mic capture until the user explicitly toggles Voice → Wake-word in Settings.
public final class WakeWord {
    public static let sentinelByteThreshold = 1024

    private let vad: VAD
    private let suppression: AppSuppression
    public private(set) var armed: Bool = false
    public var onTrigger: () -> Void = {}

    public init(vad: VAD = VAD(), suppression: AppSuppression = AppSuppression()) {
        self.vad = vad
        self.suppression = suppression
    }

    public func arm()    { armed = true }
    public func disarm() { armed = false }

    public func start() { arm() }
    public func stop()  { disarm() }

    /// simulateAudio is the test seam — production feeds AVAudioPCMBuffer frames
    /// into the CoreML model; tests pass a scalar RMS to exercise gate logic
    /// without an audio session or a trained model.
    public func simulateAudio(rms: Float) {
        guard armed else { return }
        guard !suppression.suppressedRightNow() else { return }
        guard rms >= vad.floor else { return }
        onTrigger()
    }

    /// modelReady gates detection on a real CoreML payload — until the model is
    /// trained we ship a sentinel and short-circuit anything below the threshold.
    public static func modelReady(byteCount: Int) -> Bool {
        byteCount >= sentinelByteThreshold
    }

    public static func bundledModelByteCount() -> Int {
        guard let url = Bundle.module.url(forResource: "wake-leah", withExtension: "mlmodel"),
              let attrs = try? FileManager.default.attributesOfItem(atPath: url.path),
              let size = attrs[.size] as? Int
        else { return 0 }
        return size
    }
}
