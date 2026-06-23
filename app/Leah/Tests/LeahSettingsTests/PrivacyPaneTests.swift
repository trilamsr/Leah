import XCTest
@testable import LeahUI

final class PrivacyPaneTests: XCTestCase {
    @MainActor
    func testTelemetryDefaultOff() {
        let model = PrivacyPaneModel(gate: FakeTelemetryGate(authorized: true, available: true))
        XCTAssertFalse(model.telemetry, "telemetry default = off (spec §17.13)")
    }

    @MainActor
    func testTelemetryToggleRequiresTouchID() async {
        let gate = FakeTelemetryGate(authorized: false, available: true)
        let model = PrivacyPaneModel(gate: gate)
        let outcome = await model.applyToggle(true)
        XCTAssertEqual(outcome, .biometricDenied)
        XCTAssertFalse(model.telemetry, "denied Touch ID → value unchanged")
        XCTAssertEqual(gate.evaluateCalls, 1)
    }

    @MainActor
    func testTelemetryToggleAppliesWhenTouchIDAuthorized() async {
        let gate = FakeTelemetryGate(authorized: true, available: true)
        let model = PrivacyPaneModel(gate: gate)
        let outcome = await model.applyToggle(true)
        XCTAssertEqual(outcome, .applied)
        XCTAssertTrue(model.telemetry)
    }

    @MainActor
    func testTelemetryToggleFallsThroughWhenBiometricsUnavailable() async {
        let gate = FakeTelemetryGate(authorized: false, available: false)
        let model = PrivacyPaneModel(gate: gate)
        let outcome = await model.applyToggle(true)
        XCTAssertEqual(outcome, .applied, "no biometrics → toggle applies (off-default is the friction)")
        XCTAssertTrue(model.telemetry)
        XCTAssertEqual(gate.evaluateCalls, 0)
    }
}

final class FakeTelemetryGate: TelemetryGating {
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
