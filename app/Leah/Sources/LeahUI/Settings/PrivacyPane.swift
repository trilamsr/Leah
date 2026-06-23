import SwiftUI
import LeahAuth

public enum TelemetryToggleOutcome: Equatable {
    case applied
    case biometricDenied
}

public protocol TelemetryGating {
    var biometricsAvailable: Bool { get }
    func evaluate(reason: String) async -> Bool
    func evaluateWithPasswordFallback(reason: String) async -> Bool
}

extension BiometricsGate: TelemetryGating {}

@MainActor
public final class PrivacyPaneModel: ObservableObject {
    @Published public private(set) var telemetry: Bool
    private let gate: TelemetryGating

    public init(telemetry: Bool = false, gate: TelemetryGating) {
        self.telemetry = telemetry
        self.gate = gate
    }

    /// Touch ID when available; system-password (`.deviceOwnerAuthentication`)
    /// fallback when not (spec §17.13). On denial the optimistic flip is reverted
    /// — the @Published republish drives a SwiftUI redraw so the Toggle's visual
    /// state snaps back to the source-of-truth.
    public func applyToggle(_ next: Bool) async -> TelemetryToggleOutcome {
        let prior = telemetry
        telemetry = next
        let ok: Bool = gate.biometricsAvailable
            ? await gate.evaluate(reason: "Change telemetry setting")
            : await gate.evaluateWithPasswordFallback(reason: "Change telemetry setting")
        guard ok else {
            telemetry = prior
            return .biometricDenied
        }
        return .applied
    }
}

public struct PrivacyPane: View {
    @StateObject private var model: PrivacyPaneModel
    @State private var status = ""

    public init(gate: TelemetryGating = BiometricsGate()) {
        _model = StateObject(wrappedValue: PrivacyPaneModel(gate: gate))
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Toggle(isOn: Binding(
                get: { model.telemetry },
                set: { next in
                    Task {
                        let outcome = await model.applyToggle(next)
                        status = (outcome == .biometricDenied)
                            ? "Authentication denied — telemetry unchanged."
                            : ""
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
