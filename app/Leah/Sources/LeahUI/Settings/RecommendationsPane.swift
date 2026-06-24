import SwiftUI

public struct AntiRuleRow: Identifiable, Equatable {
    public let id: String
    public let kind: String
    public let reason: String
    public let source: AntiRuleSource
    public let addedAt: Date

    public init(kind: String, reason: String, source: AntiRuleSource, addedAt: Date) {
        self.id = "\(kind)|\(source.rawValue)"
        self.kind = kind
        self.reason = reason
        self.source = source
        self.addedAt = addedAt
    }
}

public enum AntiRuleSource: String, Equatable {
    case operator_ = "operator"
    case auto
    case spec

    public var isLocked: Bool { self == .spec }
}

public struct ExperimentRow: Identifiable, Equatable {
    public let id: Int64
    public let kind: String
    public let armA: String
    public let armB: String
    public let impressionsA: Int
    public let impressionsB: Int
    public let winsA: Int
    public let winsB: Int
    public let locked: Bool
    public let lockedArm: String?

    public init(id: Int64, kind: String, armA: String, armB: String,
                impressionsA: Int, impressionsB: Int, winsA: Int, winsB: Int,
                locked: Bool, lockedArm: String?) {
        self.id = id
        self.kind = kind
        self.armA = armA
        self.armB = armB
        self.impressionsA = impressionsA
        self.impressionsB = impressionsB
        self.winsA = winsA
        self.winsB = winsB
        self.locked = locked
        self.lockedArm = lockedArm
    }
}

public struct SurfacedLedgerRow: Identifiable, Equatable {
    public let id: Int64
    public let kind: String
    public let body: String
    public let outcome: String
    public let at: Date

    public init(id: Int64, kind: String, body: String, outcome: String, at: Date) {
        self.id = id
        self.kind = kind
        self.body = body
        self.outcome = outcome
        self.at = at
    }
}

@MainActor
public final class RecommendationsPaneModel: ObservableObject {
    @Published public var kindsEnabled: [String: Bool]
    @Published public var rules: [AntiRuleRow]
    @Published public var experiments: [ExperimentRow]
    @Published public var ledger: [SurfacedLedgerRow]

    public init(kindsEnabled: [String: Bool] = [:],
                rules: [AntiRuleRow] = [],
                experiments: [ExperimentRow] = [],
                ledger: [SurfacedLedgerRow] = []) {
        self.kindsEnabled = kindsEnabled
        self.rules = rules
        self.experiments = experiments
        self.ledger = ledger
    }

    public var editableRules: [AntiRuleRow] {
        rules.filter { !$0.source.isLocked }
    }

    public var lockedRules: [AntiRuleRow] {
        rules.filter { $0.source.isLocked }
    }

    public var recentLedger: [SurfacedLedgerRow] {
        Array(ledger.prefix(30))
    }

    public func removeEditable(_ row: AntiRuleRow) -> Bool {
        guard !row.source.isLocked else { return false }
        rules.removeAll { $0.id == row.id }
        return true
    }
}

public struct RecommendationsPane: View {
    @StateObject private var model: RecommendationsPaneModel
    @State private var paneState: PaneState

    @MainActor
    public init(model: RecommendationsPaneModel? = nil, initialState: PaneState = .loaded) {
        _model = StateObject(wrappedValue: model ?? RecommendationsPaneModel())
        _paneState = State(initialValue: initialState)
    }

    public var body: some View {
        switch paneState {
        case .empty:
            EmptyState(symbol: "sparkles",
                       title: "No recommendations yet",
                       caption: "Leah surfaces suggestions after you use it for a while. Toggles and history will appear here.")
        case .error(let msg):
            ErrorState(message: msg) { paneState = .loaded }
        case .loaded:
            loaded
        }
    }

    private var loaded: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                kindToggles
                Divider()
                antiListSection
                Divider()
                experimentsSection
                Divider()
                ledgerSection
            }
            .padding(24)
        }
    }

    private var kindToggles: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Recommendation kinds").font(.system(size: 13, weight: .semibold)).foregroundColor(.white)
            ForEach(model.kindsEnabled.keys.sorted(), id: \.self) { kind in
                Toggle(isOn: Binding(
                    get: { model.kindsEnabled[kind] ?? false },
                    set: { model.kindsEnabled[kind] = $0 }
                )) {
                    Text(kind).font(.system(size: 13)).foregroundColor(.white)
                }
                .toggleStyle(.switch)
            }
        }
    }

    private var antiListSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Anti-recommend").font(.system(size: 13, weight: .semibold)).foregroundColor(.white)
            if !model.editableRules.isEmpty {
                ForEach(model.editableRules) { row in
                    HStack {
                        Text(row.kind).foregroundColor(.white).font(.system(size: 12, design: .monospaced))
                        Text(row.reason).foregroundColor(.gray).font(.system(size: 12))
                        Spacer()
                        Button("Remove") { _ = model.removeEditable(row) }
                            .buttonStyle(.borderless)
                    }
                }
            }
            ForEach(model.lockedRules) { row in
                HStack {
                    Image(systemName: "lock.fill").foregroundColor(.gray)
                    Text(row.kind).foregroundColor(.white).font(.system(size: 12, design: .monospaced))
                    Text(row.reason).foregroundColor(.gray).font(.system(size: 12))
                    Spacer()
                    Text("spec").foregroundColor(.gray).font(.system(size: 11))
                }
            }
        }
    }

    private var experimentsSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("A/B experiments").font(.system(size: 13, weight: .semibold)).foregroundColor(.white)
            ForEach(model.experiments) { exp in
                VStack(alignment: .leading, spacing: 2) {
                    HStack {
                        Text(exp.kind).foregroundColor(.white).font(.system(size: 12, design: .monospaced))
                        if exp.locked, let arm = exp.lockedArm {
                            Text("locked → \(arm)").foregroundColor(.green).font(.system(size: 11))
                        }
                    }
                    Text("A: \(exp.winsA)/\(exp.impressionsA)  ·  B: \(exp.winsB)/\(exp.impressionsB)")
                        .foregroundColor(.gray).font(.system(size: 11))
                }
            }
        }
    }

    private var ledgerSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Recent surfaced (last 30)").font(.system(size: 13, weight: .semibold)).foregroundColor(.white)
            ForEach(model.recentLedger) { row in
                HStack {
                    Text(row.kind).foregroundColor(.white).font(.system(size: 12, design: .monospaced))
                    Text(row.body).foregroundColor(.gray).font(.system(size: 12)).lineLimit(1)
                    Spacer()
                    Text(row.outcome).foregroundColor(.gray).font(.system(size: 11))
                }
            }
        }
    }
}
