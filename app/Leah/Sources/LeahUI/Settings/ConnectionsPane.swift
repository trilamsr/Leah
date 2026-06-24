import SwiftUI
import EventKit

// IntegrationRow is the outbound-integration row model (Calendar/Mail/Files).
// Kept under the old name so existing tests + import sites continue to work
// while the surrounding pane becomes "Connections" per phase4 §5.10.
public struct IntegrationRow {
    public enum Kind: String, CaseIterable, Equatable {
        case calendar, mail, files

        public var label: String {
            switch self {
            case .calendar: return "Calendar"
            case .mail:     return "Mail"
            case .files:    return "Files"
            }
        }
    }
}

extension IntegrationRow.Kind: Identifiable {
    public var id: String { rawValue }
}

// ConnectionsPane composes the three §5.10 sections: outbound integrations
// (existing v1 surface), inbound MCP tokens, A2A peers.
public struct ConnectionsPane: View {
    @State private var paneState: PaneState

    public init(initialState: PaneState = .loaded) {
        _paneState = State(initialValue: initialState)
    }

    public var body: some View {
        switch paneState {
        case .empty:
            EmptyState(symbol: "link",
                       title: "No connections yet",
                       caption: "Calendar, Mail, Files and MCP tokens appear here once you connect them.")
        case .error(let msg):
            ErrorState(message: msg) { paneState = .loaded }
        case .loaded:
            loaded
        }
    }

    private var loaded: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                section("Outbound integrations") { OutboundIntegrationsSection() }
                section("Inbound MCP")            { InboundMCPSection() }
                section("A2A peers")              { A2APeersSection() }
            }
            .padding(24)
        }
    }

    private func section<Content: View>(_ title: String, @ViewBuilder _ content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title).font(.callout.weight(.semibold)).foregroundColor(.white)
            content()
        }
    }
}

// OutboundIntegrationsSection preserves the v1 calendar/mail/files row block
// so the existing EventKit wiring keeps working under the new pane title.
struct OutboundIntegrationsSection: View {
    @State private var calendarState: ConnectionState = .none
    @State private var mailState:     ConnectionState = .none
    @State private var filesState:    ConnectionState = .none
    @State private var pendingDisconnect: IntegrationRow.Kind?
    private let store = EKEventStore()

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            row(.calendar, state: calendarState)
            Divider()
            row(.mail, state: mailState)
            Divider()
            row(.files, state: filesState)
        }
        .onAppear { refresh() }
        .alert(item: $pendingDisconnect) { kind in
            Alert(
                title: Text("Disconnect \(kind.label)?"),
                message: Text("Memory indexed from \(kind.label) will be purged."),
                primaryButton: .destructive(Text("Disconnect & purge")) { disconnect(kind) },
                secondaryButton: .cancel()
            )
        }
    }

    private func row(_ kind: IntegrationRow.Kind, state: ConnectionState) -> some View {
        HStack(spacing: 12) {
            StatusGlyphView(state: state)
            Text(kind.label).foregroundColor(.white)
            Spacer()
            if state == .connected {
                Button("Disconnect") { pendingDisconnect = kind }
            } else {
                Button("Connect") { connect(kind) }
            }
        }
    }

    private func refresh() {
        calendarState = EKEventStore.authorizationStatus(for: .event) == .fullAccess ? .connected : .none
    }

    private func connect(_ kind: IntegrationRow.Kind) {
        switch kind {
        case .calendar:
            if #available(macOS 14.0, *) {
                store.requestFullAccessToEvents { ok, _ in
                    DispatchQueue.main.async { calendarState = ok ? .connected : .none }
                }
            } else {
                store.requestAccess(to: .event) { ok, _ in
                    DispatchQueue.main.async { calendarState = ok ? .connected : .none }
                }
            }
        case .mail:  mailState = .partial
        case .files: filesState = .partial
        }
    }

    private func disconnect(_ kind: IntegrationRow.Kind) {
        switch kind {
        case .calendar: calendarState = .none
        case .mail:     mailState = .none
        case .files:    filesState = .none
        }
    }
}

// IntegrationsPane is the v1 type-alias kept so any out-of-pane import (e.g.
// the menubar wizard) keeps compiling while callers migrate. Remove after
// the T17 supervisor wave when sibling code is repointed.
public typealias IntegrationsPane = ConnectionsPane
