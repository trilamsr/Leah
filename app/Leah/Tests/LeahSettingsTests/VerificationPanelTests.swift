import XCTest
@testable import LeahUI

@MainActor
final class VerificationPanelTests: XCTestCase {
    func testStateChipColorPerState() {
        XCTAssertEqual(StateChip.background(for: .verified), .green)
        XCTAssertEqual(StateChip.background(for: .stale), .orange)
        XCTAssertEqual(StateChip.background(for: .failed), .red)
        XCTAssertEqual(StateChip.background(for: .unknown), .gray)
    }

    func testModelStartsUnknown() {
        let m = VerificationModel()
        XCTAssertEqual(m.state, .unknown)
        XCTAssertEqual(m.signers.count, 0)
        XCTAssertEqual(m.lastChecked, .distantPast)
    }

    func testApplyUpdatesPublishedFields() {
        let m = VerificationModel()
        let signer = VerificationSigner(kind: "ed25519", fingerprint: "abcdef0123456789")
        let when = Date(timeIntervalSince1970: 1_700_000_000)
        m.apply(state: .verified, signers: [signer], reason: "", at: when)
        XCTAssertEqual(m.state, .verified)
        XCTAssertEqual(m.signers, [signer])
        XCTAssertEqual(m.lastChecked, when)
    }

    func testRecheckInvokesClosure() async {
        var fired = false
        let m = VerificationModel(recheck: { fired = true })
        await m.recheck()
        XCTAssertTrue(fired)
    }

    func testAllAttestStatesCovered() {
        XCTAssertEqual(Set(AttestState.allCases), Set([.verified, .stale, .failed, .unknown]))
    }
}
