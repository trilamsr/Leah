import AVFoundation

/// VAD is the §6.7 voice-activity gate; sustained RMS above floor is what
/// keeps single-syllable fragments from firing the wake-word.
public final class VAD {
    public let floor: Float
    public init(floor: Float = 0.05) { self.floor = floor }

    public func gateOpens(buffer: AVAudioPCMBuffer) -> Bool {
        guard let chs = buffer.floatChannelData else { return false }
        let n = Int(buffer.frameLength)
        guard n > 0 else { return false }
        let ptr = chs[0]
        var sumSq: Float = 0
        for i in 0..<n { sumSq += ptr[i] * ptr[i] }
        let rms = (sumSq / Float(n)).squareRoot()
        return rms >= floor
    }
}
