import XCTest
import AppKit
@testable import LeahUI

final class PushToTalkTests: XCTestCase {
    func test_FnPressFiresOnStart() {
        var started = false
        let ptt = PushToTalk()
        ptt.onStart = { started = true }
        ptt.simulate(modifierFlags: .function, keyCode: 0)
        XCTAssertTrue(started)
    }

    func test_RightCommandFiresOnStart() {
        var started = false
        let ptt = PushToTalk()
        ptt.onStart = { started = true }
        // kVK_RightCommand = 54
        ptt.simulate(modifierFlags: .command, keyCode: 54)
        XCTAssertTrue(started)
    }

    func test_SpaceDoesNotFirePTT() {
        var started = false
        let ptt = PushToTalk()
        ptt.onStart = { started = true }
        // kVK_Space = 49 — must NOT trigger PTT.
        ptt.simulate(modifierFlags: [], keyCode: 49)
        XCTAssertFalse(started)
    }

    func test_OptionDoesNotFirePTT() {
        var started = false
        let ptt = PushToTalk()
        ptt.onStart = { started = true }
        // Option alone collides with ⌥Space global hotkey — must NOT trigger PTT.
        ptt.simulate(modifierFlags: .option, keyCode: 0)
        XCTAssertFalse(started)
    }

    func test_ReleaseAfterFnFiresOnStop() {
        var stopped = false
        let ptt = PushToTalk()
        ptt.onStop = { stopped = true }
        ptt.simulate(modifierFlags: .function, keyCode: 0)
        ptt.simulate(modifierFlags: [], keyCode: 0)
        XCTAssertTrue(stopped)
    }

    func test_LeftCommandDoesNotFirePTT() {
        var started = false
        let ptt = PushToTalk()
        ptt.onStart = { started = true }
        // kVK_Command (left) = 55 — only right-⌘ (54) qualifies per §6.4.
        ptt.simulate(modifierFlags: .command, keyCode: 55)
        XCTAssertFalse(started)
    }

    func test_RepeatedHoldDoesNotRefire() {
        var startCount = 0
        let ptt = PushToTalk()
        ptt.onStart = { startCount += 1 }
        ptt.simulate(modifierFlags: .function, keyCode: 0)
        ptt.simulate(modifierFlags: .function, keyCode: 0)
        XCTAssertEqual(startCount, 1)
    }
}
