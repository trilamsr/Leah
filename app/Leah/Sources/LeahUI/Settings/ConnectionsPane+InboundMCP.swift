import SwiftUI

// InboundMCPScope mirrors the Go inbound.Scope closed set (phase4 §5.3). The
// raw strings MUST match the Go constants — the operator-facing checkbox
// labels render off `.label`.
public enum InboundMCPScope: String, CaseIterable, Identifiable, Equatable {
    case memoryRead   = "memory:read"
    case calendarRead = "calendar:read"
    case repoRead     = "repo:read"
    case askRun       = "ask:run"
    case widgetRender = "widget:render"

    public var id: String { rawValue }

    public var label: String {
        switch self {
        case .memoryRead:   return "Memory read"
        case .calendarRead: return "Calendar read"
        case .repoRead:     return "Repo read"
        case .askRun:       return "Ask (charges Anthropic budget)"
        case .widgetRender: return "Widget render"
        }
    }
}

// InboundMCPToken is the UI projection of an mcp_token row. `plain` is set
// ONLY by the issue handler and cleared immediately after the operator
// dismisses the reveal sheet (§5.7 — plain shown once).
public struct InboundMCPToken: Identifiable, Equatable {
    public let id: Int64
    public let name: String
    public let scopes: [InboundMCPScope]
    public let issuedAt: Date
    public var plain: String?
    public var revokedAt: Date?

    public init(id: Int64, name: String, scopes: [InboundMCPScope], issuedAt: Date, plain: String? = nil, revokedAt: Date? = nil) {
        self.id = id
        self.name = name
        self.scopes = scopes
        self.issuedAt = issuedAt
        self.plain = plain
        self.revokedAt = revokedAt
    }
}

// InboundMCPTokenStore is the section's backing store. Production wires a
// type that round-trips through the Go inbound.TokenStore over IPC; the
// in-memory `PreviewInboundMCPTokenStore` below is used by previews + tests.
public protocol InboundMCPTokenStore: AnyObject {
    func listTokens() -> [InboundMCPToken]
    func issue(name: String, scopes: [InboundMCPScope]) -> InboundMCPToken
    func revoke(id: Int64)
}

public final class PreviewInboundMCPTokenStore: InboundMCPTokenStore {
    private var rows: [InboundMCPToken] = []
    private var nextID: Int64 = 0
    public init() {}

    public func listTokens() -> [InboundMCPToken] { rows.filter { $0.revokedAt == nil } }

    public func issue(name: String, scopes: [InboundMCPScope]) -> InboundMCPToken {
        nextID += 1
        let plain = String((0..<64).map { _ in "0123456789abcdef".randomElement()! })
        let t = InboundMCPToken(id: nextID, name: name, scopes: scopes, issuedAt: Date(), plain: plain)
        rows.append(t)
        return t
    }

    public func revoke(id: Int64) {
        for i in rows.indices where rows[i].id == id {
            rows[i].revokedAt = Date()
        }
    }
}

// InboundMCPSection drives the §5.10 token table. Issue → reveal-once sheet
// → revoke. The Section reads from `store` so the UI surface stays trivial
// to unit-test against an in-memory PreviewInboundMCPTokenStore.
public struct InboundMCPSection: View {
    @State private var enabled: Bool = false
    @State private var tokens: [InboundMCPToken] = []
    @State private var revealing: InboundMCPToken?
    @State private var newName: String = ""
    @State private var newScopes: Set<InboundMCPScope> = [.memoryRead]
    private let store: InboundMCPTokenStore

    public init(store: InboundMCPTokenStore = PreviewInboundMCPTokenStore()) {
        self.store = store
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Toggle("Allow inbound MCP", isOn: $enabled)
                .foregroundColor(.white)
            if enabled {
                tokenTable
                Divider()
                issueForm
            } else {
                Text("Tokens are inactive while inbound MCP is off.")
                    .font(.system(size: 12))
                    .foregroundColor(.gray)
            }
        }
        .onAppear { tokens = store.listTokens() }
        .sheet(item: $revealing) { tok in
            revealSheet(tok)
        }
    }

    private var tokenTable: some View {
        VStack(alignment: .leading, spacing: 6) {
            if tokens.isEmpty {
                Text("No tokens issued.").font(.system(size: 12)).foregroundColor(.gray)
            } else {
                ForEach(tokens) { t in
                    HStack(spacing: 12) {
                        Text(t.name).foregroundColor(.white)
                        Text(t.scopes.map { $0.rawValue }.joined(separator: ", "))
                            .font(.system(size: 11)).foregroundColor(.gray)
                        Spacer()
                        Button("Revoke") {
                            store.revoke(id: t.id)
                            tokens = store.listTokens()
                        }
                    }
                }
            }
        }
    }

    private var issueForm: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Issue token").font(.system(size: 12, weight: .semibold)).foregroundColor(.white)
            TextField("name (e.g. ci-bot)", text: $newName)
                .textFieldStyle(.roundedBorder)
            ForEach(InboundMCPScope.allCases) { sc in
                Toggle(sc.label, isOn: Binding(
                    get: { newScopes.contains(sc) },
                    set: { on in
                        if on { newScopes.insert(sc) } else { newScopes.remove(sc) }
                    }
                ))
                .font(.system(size: 11))
                .foregroundColor(.white)
            }
            Button("Issue") {
                guard !newName.isEmpty, !newScopes.isEmpty else { return }
                let t = store.issue(name: newName, scopes: Array(newScopes).sorted { $0.rawValue < $1.rawValue })
                tokens = store.listTokens()
                revealing = t
                newName = ""
                newScopes = [.memoryRead]
            }
            .disabled(newName.isEmpty || newScopes.isEmpty)
        }
    }

    private func revealSheet(_ tok: InboundMCPToken) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Copy this token now").font(.system(size: 13, weight: .semibold))
            Text("It will not be shown again.").font(.system(size: 11)).foregroundColor(.gray)
            ScrollView(.horizontal) {
                Text(tok.plain ?? "")
                    .font(.system(.body, design: .monospaced))
                    .textSelection(.enabled)
            }
            Button("Done") { revealing = nil }
        }
        .padding(24)
        .frame(minWidth: 480)
    }
}
