import SwiftUI

public enum AttestState: String, CaseIterable {
    case verified = "Verified"
    case stale = "Stale"
    case failed = "Failed"
    case unknown = "Unknown"
}

public struct VerificationSigner: Hashable {
    public let kind: String
    public let fingerprint: String
    public init(kind: String, fingerprint: String) {
        self.kind = kind
        self.fingerprint = fingerprint
    }
}

@MainActor
public final class VerificationModel: ObservableObject {
    @Published public var state: AttestState
    @Published public var lastChecked: Date
    @Published public var signers: [VerificationSigner]
    @Published public var reason: String

    private let recheckAction: () async -> Void

    public init(
        state: AttestState = .unknown,
        lastChecked: Date = .distantPast,
        signers: [VerificationSigner] = [],
        reason: String = "",
        recheck: @escaping () async -> Void = {}
    ) {
        self.state = state
        self.lastChecked = lastChecked
        self.signers = signers
        self.reason = reason
        self.recheckAction = recheck
    }

    public func recheck() async {
        await recheckAction()
    }

    public func apply(state: AttestState, signers: [VerificationSigner], reason: String, at: Date) {
        self.state = state
        self.signers = signers
        self.reason = reason
        self.lastChecked = at
    }
}

public struct StateChip: View {
    public let state: AttestState
    public init(state: AttestState) { self.state = state }
    public var body: some View {
        Text(state.rawValue)
            .font(.caption.bold())
            .padding(.horizontal, 8)
            .padding(.vertical, 3)
            .background(Self.background(for: state))
            .foregroundColor(.white)
            .clipShape(Capsule())
            .accessibilityIdentifier("attest.chip.\(state.rawValue)")
    }

    public static func background(for state: AttestState) -> Color {
        switch state {
        case .verified: return .green
        case .stale: return .orange
        case .failed: return .red
        case .unknown: return .gray
        }
    }
}

public struct VerificationPanel: View {
    @ObservedObject public var model: VerificationModel

    public init(model: VerificationModel) {
        self.model = model
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                StateChip(state: model.state)
                Text(lastCheckedLabel)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            ForEach(model.signers, id: \.fingerprint) { signer in
                Text("\(signer.kind): \(signer.fingerprint.prefix(12))…")
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundColor(.gray)
            }
            if !model.reason.isEmpty {
                Text(model.reason)
                    .font(.caption)
                    .foregroundColor(.red)
            }
            Button("Recheck now") {
                Task { await model.recheck() }
            }
        }
    }

    private var lastCheckedLabel: String {
        if model.lastChecked == .distantPast {
            return "Never checked"
        }
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .full
        return "Last checked " + formatter.localizedString(for: model.lastChecked, relativeTo: Date())
    }
}
