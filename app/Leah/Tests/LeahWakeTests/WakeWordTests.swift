import AVFoundation
import XCTest
@testable import LeahWake

final class WakeWordTests: XCTestCase {
    func testArmedDefaultFalse() {
        var fired = false
        let w = WakeWord()
        w.onTrigger = { fired = true }
        w.simulateAudio(rms: 0.9)
        XCTAssertFalse(fired)
    }

    func testDisarmedDoesNotFire() {
        var fired = false
        let w = WakeWord()
        w.onTrigger = { fired = true }
        w.arm()
        w.disarm()
        w.simulateAudio(rms: 0.9)
        XCTAssertFalse(fired)
    }

    func testArmedFiresWhenAboveVADAndNotSuppressed() {
        var fired = false
        let w = WakeWord(suppression: AppSuppression(stubFrontmost: "com.apple.Terminal"))
        w.onTrigger = { fired = true }
        w.arm()
        w.simulateAudio(rms: 0.9)
        XCTAssertTrue(fired)
    }

    func testArmedSuppressedInZoom() {
        var fired = false
        let w = WakeWord(suppression: AppSuppression(stubFrontmost: "us.zoom.xos"))
        w.onTrigger = { fired = true }
        w.arm()
        w.simulateAudio(rms: 0.9)
        XCTAssertFalse(fired)
    }

    func testSentinelModelShortCircuits() {
        XCTAssertFalse(WakeWord.modelReady(byteCount: 0))
        XCTAssertFalse(WakeWord.modelReady(byteCount: 1))
        XCTAssertFalse(WakeWord.modelReady(byteCount: 1023))
        XCTAssertTrue(WakeWord.modelReady(byteCount: 1024))
        XCTAssertTrue(WakeWord.modelReady(byteCount: 1_048_576))
    }
}
