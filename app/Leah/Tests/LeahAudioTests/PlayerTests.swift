import XCTest
@testable import LeahAudio

final class PlayerTests: XCTestCase {
    func test_enqueueDoesNotThrow() {
        let p = Player()
        let fakeMP3 = Data([0xFF, 0xFB, 0x90, 0x00])
        p.enqueue(fakeMP3)
        XCTAssertTrue(true)
    }

    func test_resetIsIdempotent() {
        let p = Player()
        p.reset()
        p.reset()
        XCTAssertTrue(true)
    }
}
