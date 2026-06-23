import SwiftUI
import LeahAuth
import LocalAuthentication

public enum TelemetryToggleOutcome: Equatable {
    case applied
    case biometricDenied
}

public protocol TelemetryGating {
    var biometricsAvailable: Bool { get }
    func evaluate(reason: String) async -> Bool
}

public final class TelemetryGate: TelemetryGating {
    private let guard_: TouchIDGuard
    public init(guard_: TouchIDGuard = TouchIDGuard()) { self.guard_ = guard_ }

    public var biometricsAvailable: Bool {
        var err: NSError?
        return LAContext().canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err)
    }

    public func evaluate(reason: String) async -> Bool {
        await guard_.confirm(reason: reason)
    }
}

@MainActor
public final class PrivacyPaneModel: ObservableObject {
    @Published public private(set) var telemetry: Bool
    private let gate: TelemetryGating

    public init(telemetry: Bool = false, gate: TelemetryGating) {
        self.telemetry = telemetry
        self.gate = gate
    }

    /// Apply a telemetry toggle. Touch ID gates the change when biometrics are
    /// available; on denial the value is NOT mutated. When biometrics are
    /// unavailable the toggle still applies (typed-friction equivalent here is
    /// the off-default + visible reveal of the toggle).
    public func applyToggle(_ next: Bool) async -> TelemetryToggleOutcome {
        if gate.biometricsAvailable {
            let ok = await gate.evaluate(reason: "Change telemetry setting")
            guard ok else { return .biometricDenied }
        }
        telemetry = next
        return .applied
    }
}

public struct PrivacyPane: View {
    @StateObject private var model: PrivacyPaneModel
    @State private var status = ""

    public init(gate: TelemetryGating = TelemetryGate()) {
        _model = StateObject(wrappedValue: PrivacyPaneModel(gate: gate))
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Toggle(isOn: Binding(
                get: { model.telemetry },
                set: { next in
                    Task {
                        let outcome = await model.applyToggle(next)
                        if outcome == .biometricDenied {
                            status = "Touch ID denied — telemetry unchanged."
                        } else {
                            status = ""
                        }
                    }
                }
            )) {
                Text("Send anonymous error reports + performance metrics. Never includes message content.")
                    .font(.system(size: 13))
                    .foregroundColor(.white)
            }
            if !status.isEmpty {
                Text(status).font(.system(size: 12)).foregroundColor(.gray)
            }
            Divider()
            Text("Embed locally (slower, private) — Phase 2")
                .font(.system(size: 13)).foregroundColor(.gray)
            Divider()
            Text("Conversation history kept: 24 hours (locked Phase 1)")
                .font(.system(size: 13)).foregroundColor(.white)
            Spacer()
        }
        .padding(24)
    }
}
