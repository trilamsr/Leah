import SwiftUI
import LeahAuth
import LocalAuthentication

public struct MemoryStats: Equatable {
    public let chunkCount: Int
    public let modelID: String
    public let dim: Int

    public init(chunkCount: Int, modelID: String, dim: Int) {
        self.chunkCount = chunkCount
        self.modelID = modelID
        self.dim = dim
    }

    public var tableName: String {
        let safe = modelID.replacingOccurrences(of: "-", with: "_").replacingOccurrences(of: ".", with: "_")
        return "embeddings_\(safe)_\(dim)"
    }

    public var modelDimLabel: String { "\(modelID) · \(dim)d" }
}

public enum PurgeOutcome: Equatable {
    case authorized
    case typedFrictionFailed
    case biometricDenied
}

public protocol TouchIDGating {
    var biometricsAvailable: Bool { get }
    func evaluate(reason: String) async -> Bool
}

public final class TouchIDGate: TouchIDGating {
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
public final class MemoryPaneModel: ObservableObject {
    @Published public var stats: MemoryStats
    private let gate: TouchIDGating

    public init(stats: MemoryStats, gate: TouchIDGating) {
        self.stats = stats
        self.gate = gate
    }

    public func attemptPurge(typed: String) async -> PurgeOutcome {
        guard typed == "PURGE" else { return .typedFrictionFailed }
        if gate.biometricsAvailable {
            let ok = await gate.evaluate(reason: "Purge all Leah memory")
            return ok ? .authorized : .biometricDenied
        }
        return .authorized
    }
}

public struct MemoryPane: View {
    @StateObject private var model: MemoryPaneModel
    @State private var typed = ""
    @State private var status = ""

    public init(stats: MemoryStats = MemoryStats(chunkCount: 0, modelID: "voyage-3.5-lite", dim: 1024),
                gate: TouchIDGating = TouchIDGate()) {
        _model = StateObject(wrappedValue: MemoryPaneModel(stats: stats, gate: gate))
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            row("Total chunks", value: "\(model.stats.chunkCount)")
            Divider()
            row("Embedding model", value: model.stats.modelDimLabel)
            Divider()
            HStack {
                Text("Active table").foregroundColor(.gray).font(.system(size: 13))
                Spacer()
                Text(model.stats.tableName)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundColor(.gray)
                    .textSelection(.enabled)
            }
            Divider()
            VStack(alignment: .leading, spacing: 8) {
                Text("Type PURGE then press Purge to delete all memory.")
                    .font(.system(size: 12)).foregroundColor(.gray)
                HStack {
                    TextField("", text: $typed)
                        .textFieldStyle(.roundedBorder)
                        .frame(width: 160)
                    Button("Purge all memory") {
                        Task {
                            let outcome = await model.attemptPurge(typed: typed)
                            status = label(for: outcome)
                            if outcome == .authorized { typed = "" }
                        }
                    }
                    .disabled(typed != "PURGE")
                }
                if !status.isEmpty {
                    Text(status).font(.system(size: 12)).foregroundColor(.gray)
                }
            }
            Spacer()
        }
        .padding(24)
    }

    private func row(_ label: String, value: String) -> some View {
        HStack {
            Text(label).foregroundColor(.gray).font(.system(size: 13))
            Spacer()
            Text(value).foregroundColor(.white).font(.system(size: 13))
        }
    }

    private func label(for outcome: PurgeOutcome) -> String {
        switch outcome {
        case .authorized:          return "Purged."
        case .typedFrictionFailed: return "Type PURGE exactly."
        case .biometricDenied:     return "Touch ID denied — purge aborted."
        }
    }
}
