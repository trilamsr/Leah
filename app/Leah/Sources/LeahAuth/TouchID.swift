import Foundation
import LocalAuthentication

/// Wraps LAContext for sensitive ops per §17.13. Touch ID is ADDITIVE —
/// callers still require their primary friction (typed PURGE for memory, the
/// telemetry off-default for privacy) so a screenshot-of-screen biometric leak
/// can't single-handedly authorize.
public final class TouchIDGuard {
    private let stubMode: (available: Bool, succeeds: Bool)?
    private(set) public var evaluatePolicyCalls = 0

    public init() {
        self.stubMode = nil
    }

    private init(stub: (Bool, Bool)) {
        self.stubMode = stub
    }

    /// Test seam — returns a guard whose `canEvaluatePolicy` + `evaluatePolicy`
    /// outcomes are predetermined. Production code uses `init()`.
    public static func stub(available: Bool, succeeds: Bool) -> TouchIDGuard {
        TouchIDGuard(stub: (available, succeeds))
    }

    /// Returns true iff biometrics are available AND the user authorized.
    /// Returns false on hardware-unavailable; caller is responsible for the
    /// additive friction (typed PURGE etc.) that still gates the action.
    public func confirm(reason: String) async -> Bool {
        if let s = stubMode {
            guard s.available else { return false }
            evaluatePolicyCalls += 1
            return s.succeeds
        }
        let ctx = LAContext()
        var err: NSError?
        guard ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err) else {
            return false
        }
        evaluatePolicyCalls += 1
        return await withCheckedContinuation { (cont: CheckedContinuation<Bool, Never>) in
            ctx.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: reason) { ok, _ in
                cont.resume(returning: ok)
            }
        }
    }
}
