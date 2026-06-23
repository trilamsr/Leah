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
    func testTelemetry_FallsBackToDeviceOwnerAuth_WhenBiometricsUnavailable() async {
        // Spec §17.13: machines without Touch ID get a system-password prompt,
        // NOT a silent auto-apply.
        let gate = FakeTelemetryGate(authorized: false, available: false)
        let model = PrivacyPaneModel(gate: gate)
        let outcome = await model.applyToggle(true)
        XCTAssertEqual(outcome, .biometricDenied, "no biometrics + denied password → not applied")
        XCTAssertFalse(model.telemetry, "denied auth → value unchanged")
        XCTAssertEqual(gate.passwordEvaluateCalls, 1, "system-password fallback must fire when biometrics absent")
    }

    @MainActor
    func testTelemetry_FallbackAuthorizes_WhenBiometricsUnavailableButPasswordOK() async {
        let gate = FakeTelemetryGate(authorized: true, available: false)
        let model = PrivacyPaneModel(gate: gate)
        let outcome = await model.applyToggle(true)
        XCTAssertEqual(outcome, .applied)
        XCTAssertTrue(model.telemetry)
        XCTAssertEqual(gate.passwordEvaluateCalls, 1)
    }

    @MainActor
    func testTelemetry_TogglesBackOnDenial() async {
        // UI revert: SwiftUI's Toggle caches the bool until @Published republishes
        // the source-of-truth. The model must emit a redraw signal on denial so
        // the visual snaps back.
        let gate = FakeTelemetryGate(authorized: false, available: true)
        let model = PrivacyPaneModel(telemetry: false, gate: gate)
        var redraws = 0
        let cancellable = model.objectWillChange.sink { _ in redraws += 1 }
        defer { cancellable.cancel() }
        _ = await model.applyToggle(true)
        XCTAssertFalse(model.telemetry)
        XCTAssertGreaterThanOrEqual(redraws, 1, "denial must publish a change so Toggle redraws to source-of-truth")
    }
}

import Combine

final class FakeTelemetryGate: TelemetryGating {
    let authorized: Bool
    let available: Bool
    private(set) var evaluateCalls = 0
    private(set) var passwordEvaluateCalls = 0

    init(authorized: Bool, available: Bool) {
        self.authorized = authorized
        self.available = available
    }

    var biometricsAvailable: Bool { available }

    func evaluate(reason: String) async -> Bool {
        evaluateCalls += 1
        return authorized
    }

    func evaluateWithPasswordFallback(reason: String) async -> Bool {
        passwordEvaluateCalls += 1
        return authorized
    }
}
