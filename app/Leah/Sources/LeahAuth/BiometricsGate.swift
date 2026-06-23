import Foundation
import LocalAuthentication

/// Production adapter wrapping `TouchIDGuard` + an `LAContext` availability probe.
/// One concrete type for every Settings pane that needs Touch ID gating —
/// see `TelemetryGating` / `TouchIDGating` test protocols.
public final class BiometricsGate {
    private let guard_: TouchIDGuard

    public init(guard_: TouchIDGuard = TouchIDGuard()) {
        self.guard_ = guard_
    }

    public var biometricsAvailable: Bool {
        var err: NSError?
        return LAContext().canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err)
    }

    public func evaluate(reason: String) async -> Bool {
        await guard_.confirm(reason: reason)
    }

    public func evaluateWithPasswordFallback(reason: String) async -> Bool {
        await guard_.confirmWithPasswordFallback(reason: reason)
    }
}
