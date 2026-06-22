import XCTest
@testable import LeahUI

final class MemoryPaneTests: XCTestCase {
    func testMemoryShowsModelIdDim() {
        let stats = MemoryStats(chunkCount: 1247, modelID: "voyage-3.5-lite", dim: 1024)
        XCTAssertEqual(stats.tableName, "embeddings_voyage_3_5_lite_1024")
        XCTAssertEqual(stats.modelDimLabel, "voyage-3.5-lite · 1024d")
    }

    func testPurgeRequiresTouchID() async {
        let gate = FakeTouchIDGate(authorized: false, available: true)
        let pane = MemoryPaneModel(stats: .init(chunkCount: 0, modelID: "x", dim: 0), gate: gate)
        let outcome = await pane.attemptPurge(typed: "PURGE")
        XCTAssertEqual(outcome, .biometricDenied)
        XCTAssertEqual(gate.evaluateCalls, 1)
    }

    func testPurgeRequiresTypedPurge() async {
        let gate = FakeTouchIDGate(authorized: true, available: true)
        let pane = MemoryPaneModel(stats: .init(chunkCount: 0, modelID: "x", dim: 0), gate: gate)
        let outcome = await pane.attemptPurge(typed: "purge")
        XCTAssertEqual(outcome, .typedFrictionFailed)
        XCTAssertEqual(gate.evaluateCalls, 0, "typed-friction guards Touch ID — gate must not be invoked on mismatch")
    }

    func testPurgeProceedsWhenTouchIDAuthorized() async {
        let gate = FakeTouchIDGate(authorized: true, available: true)
        let pane = MemoryPaneModel(stats: .init(chunkCount: 0, modelID: "x", dim: 0), gate: gate)
        let outcome = await pane.attemptPurge(typed: "PURGE")
        XCTAssertEqual(outcome, .authorized)
    }

    func testPurgeFallsBackWhenTouchIDUnavailable() async {
        let gate = FakeTouchIDGate(authorized: false, available: false)
        let pane = MemoryPaneModel(stats: .init(chunkCount: 0, modelID: "x", dim: 0), gate: gate)
        let outcome = await pane.attemptPurge(typed: "PURGE")
        XCTAssertEqual(outcome, .authorized, "no biometrics → typed PURGE alone proceeds (spec §17.13)")
    }

    func testSettingsPaneIncludesMemory() {
        XCTAssertTrue(SettingsPane.allCases.contains(.memory))
    }
}

final class FakeTouchIDGate: TouchIDGating {
    let authorized: Bool
    let available: Bool
    private(set) var evaluateCalls = 0

    init(authorized: Bool, available: Bool) {
        self.authorized = authorized
        self.available = available
    }

    var biometricsAvailable: Bool { available }

    func evaluate(reason: String) async -> Bool {
        evaluateCalls += 1
        return authorized
    }
}
