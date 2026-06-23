import XCTest
@testable import LeahAuth

final class TouchIDTests: XCTestCase {
    func testEvaluateFiresLAContextEvaluatePolicy() async {
        let g = TouchIDGuard.stub(available: true, succeeds: true)
        let ok = await g.confirm(reason: "purge")
        XCTAssertTrue(ok)
        XCTAssertEqual(g.evaluatePolicyCalls, 1, "evaluatePolicy must be invoked when biometrics available")
    }

    func testFallbackToTypedPurgeWhenBiometricsUnavailable() async {
        let g = TouchIDGuard.stub(available: false, succeeds: false)
        let ok = await g.confirm(reason: "purge")
        XCTAssertFalse(ok, "unavailable biometrics → confirm returns false; caller falls back to typed PURGE alone")
        XCTAssertEqual(g.evaluatePolicyCalls, 0, "evaluatePolicy must NOT fire when canEvaluatePolicy=false")
    }

    func testTelemetryToggleGatedByTouchID() async {
        let denied = TouchIDGuard.stub(available: true, succeeds: false)
        let deniedOk = await denied.confirm(reason: "telemetry")
        XCTAssertFalse(deniedOk)

        let allowed = TouchIDGuard.stub(available: true, succeeds: true)
        let allowedOk = await allowed.confirm(reason: "telemetry")
        XCTAssertTrue(allowedOk)
    }
}
