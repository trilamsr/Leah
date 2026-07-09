import XCTest
@testable import LeahVision

final class ConsentSheetTests: XCTestCase {
    func testDefaultDeniedForUnknownMode() {
        let ledger = SessionConsentLedger()
        XCTAssertFalse(ledger.granted(mode: "screenshot"))
    }

    func testGrantSessionScopeRegisters() {
        let ledger = SessionConsentLedger()
        ledger.record(mode: "screenshot", choice: .alwaysThisSession)
        XCTAssertTrue(ledger.granted(mode: "screenshot"))
    }

    func testOnceScopeDoesNotPersist() {
        let ledger = SessionConsentLedger()
        ledger.record(mode: "live_screen", choice: .sendOnce)
        XCTAssertFalse(ledger.granted(mode: "live_screen"))
    }

    func testDenyDoesNotGrant() {
        let ledger = SessionConsentLedger()
        ledger.record(mode: "live_camera", choice: .keepLocal)
        XCTAssertFalse(ledger.granted(mode: "live_camera"))
    }

    func testRequestPromptsOnlyWhenUngranted() async throws {
        let ledger = SessionConsentLedger()
        var promptCount = 0
        let gate = ConsentGate(ledger: ledger) { _ in
            promptCount += 1
            return .alwaysThisSession
        }
        _ = await gate.request(mode: "screenshot")
        _ = await gate.request(mode: "screenshot")
        XCTAssertEqual(promptCount, 1)
    }

    func testRequestReturnsFalseOnKeepLocal() async throws {
        let ledger = SessionConsentLedger()
        let gate = ConsentGate(ledger: ledger) { _ in .keepLocal }
        let ok = await gate.request(mode: "screenshot")
        XCTAssertFalse(ok)
        XCTAssertFalse(ledger.granted(mode: "screenshot"))
    }

    func testRequestReturnsTrueOnSendOnce() async throws {
        let ledger = SessionConsentLedger()
        let gate = ConsentGate(ledger: ledger) { _ in .sendOnce }
        let ok = await gate.request(mode: "screenshot")
        XCTAssertTrue(ok)
        XCTAssertFalse(ledger.granted(mode: "screenshot"))
    }

    func testRequestReturnsTrueOnAlwaysThisSession() async throws {
        let ledger = SessionConsentLedger()
        let gate = ConsentGate(ledger: ledger) { _ in .alwaysThisSession }
        let ok = await gate.request(mode: "live_screen")
        XCTAssertTrue(ok)
        XCTAssertTrue(ledger.granted(mode: "live_screen"))
    }

    func testCopyForModeMatchesSpec() {
        XCTAssertEqual(ConsentSheet.title(forMode: "screenshot"), "Send this screenshot to the cloud?")
        XCTAssertEqual(ConsentSheet.title(forMode: "live_screen"), "Stream your screen to the cloud?")
        XCTAssertEqual(ConsentSheet.title(forMode: "live_camera"), "Stream your camera to the cloud?")
    }
}
