import AVFoundation
import XCTest
@testable import LeahWake

final class VADTests: XCTestCase {
    func testSilenceRejected() {
        let vad = VAD()
        let f = AVAudioFormat(standardFormatWithSampleRate: 16000, channels: 1)!
        let buf = AVAudioPCMBuffer(pcmFormat: f, frameCapacity: 256)!
        buf.frameLength = 256
        XCTAssertFalse(vad.gateOpens(buffer: buf))
    }

    func testLoudOpensGate() {
        let vad = VAD()
        let f = AVAudioFormat(standardFormatWithSampleRate: 16000, channels: 1)!
        let buf = AVAudioPCMBuffer(pcmFormat: f, frameCapacity: 256)!
        buf.frameLength = 256
        let ptr = buf.floatChannelData![0]
        for i in 0..<256 { ptr[i] = 0.5 }
        XCTAssertTrue(vad.gateOpens(buffer: buf))
    }
}
