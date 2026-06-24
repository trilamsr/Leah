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
    @State private var paneState: PaneState

    public init(gate: TelemetryGating = BiometricsGate(), initialState: PaneState = .loaded) {
        _model = StateObject(wrappedValue: PrivacyPaneModel(gate: gate))
        _paneState = State(initialValue: initialState)
    }

    public var body: some View {
        switch paneState {
        case .empty:
            EmptyState(symbol: "lock.shield",
                       title: "Privacy controls unavailable",
                       caption: "Telemetry + retention settings appear once Leah finishes setup.")
        case .error(let msg):
            ErrorState(message: msg) { paneState = .loaded }
        case .loaded:
            loaded
        }
    }

    private var loaded: some View {
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
                    .font(.callout)
                    .foregroundColor(.white)
            }
            .accessibilityLabel("Send anonymous error reports and performance metrics")
            .accessibilityHint("Never includes message content.")
            if !status.isEmpty {
                Text(status).font(.caption).foregroundColor(.gray)
            }
            Divider()
            Text("Embed locally (slower, private) — Phase 2")
                .font(.callout).foregroundColor(.gray)
            Divider()
            Text("Conversation history kept: 24 hours (locked Phase 1)")
                .font(.callout).foregroundColor(.white)
            Spacer()
        }
        .padding(24)
    }
}
