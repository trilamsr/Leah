import AppKit
import SwiftUI

/// Operator view of an installed plugin row. Mirrors leahplugin.PluginInfo on the Go side —
/// keep field names aligned so the JSON the daemon emits maps with synthesized Codable.
public struct PluginRow: Equatable, Identifiable, Hashable, Codable {
    public let id: String
    public let name: String
    public let version: String
    public let enabled: Bool
    public let attestState: String

    public init(id: String, name: String, version: String, enabled: Bool, attestState: String) {
        self.id = id
        self.name = name
        self.version = version
        self.enabled = enabled
        self.attestState = attestState
    }
}

/// Operator actions the pane forwards to the daemon. The view owns no daemon dependency directly so
/// tests drive it with a stub closure set.
public struct PluginsPaneActions {
    public var list:      () -> [PluginRow]
    public var enable:    (String) -> Void
    public var disable:   (String) -> Void
    public var uninstall: (String) -> Void
    public var install:   (URL) -> Void
    public var logs:      (String) -> Void

    public init(
        list: @escaping () -> [PluginRow],
        enable: @escaping (String) -> Void,
        disable: @escaping (String) -> Void,
        uninstall: @escaping (String) -> Void,
        install: @escaping (URL) -> Void,
        logs: @escaping (String) -> Void
    ) {
        self.list = list
        self.enable = enable
        self.disable = disable
        self.uninstall = uninstall
        self.install = install
        self.logs = logs
    }

    /// Fallback wiring used when no daemon transport is attached yet — every closure is a no-op
    /// so the pane can render in dev without crashing.
    public static let stub = PluginsPaneActions(
        list: { [] },
        enable: { _ in },
        disable: { _ in },
        uninstall: { _ in },
        install: { _ in },
        logs: { _ in }
    )
}

public struct PluginsPane: View {
    @State private var rows: [PluginRow] = []
    @State private var pendingUninstall: PluginRow?
    private let actions: PluginsPaneActions

    public init(actions: PluginsPaneActions = .stub) {
        self.actions = actions
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Plugins").font(.system(size: 16, weight: .semibold))
                Spacer()
                Button("Install…") { runInstall() }
            }
            if rows.isEmpty {
                Text("No plugins installed.")
                    .foregroundColor(.gray)
                    .padding(.top, 24)
            } else {
                ForEach(rows) { row in
                    pluginRow(row)
                    Divider()
                }
            }
            Spacer()
        }
        .padding(24)
        .onAppear { rows = actions.list() }
        .alert(item: $pendingUninstall) { row in
            Alert(
                title: Text("Uninstall \(row.name)?"),
                message: Text("Plugin logs and per-plugin scratch will be removed."),
                primaryButton: .destructive(Text("Uninstall")) {
                    actions.uninstall(row.id)
                    rows = actions.list()
                },
                secondaryButton: .cancel()
            )
        }
    }

    private func pluginRow(_ row: PluginRow) -> some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(row.name).foregroundColor(.white)
                    Text(row.version).foregroundColor(.gray).font(.system(size: 11))
                    attestBadge(row.attestState)
                }
                Text(row.id).foregroundColor(.gray).font(.system(size: 11))
            }
            Spacer()
            Button("Logs") { actions.logs(row.id) }
            if row.enabled {
                Button("Disable") {
                    actions.disable(row.id)
                    rows = actions.list()
                }
            } else {
                Button("Enable") {
                    actions.enable(row.id)
                    rows = actions.list()
                }
            }
            Button("Uninstall") { pendingUninstall = row }
                .foregroundColor(.red)
        }
    }

    /// Attestation badge tracks internal/attest.Verifier states (good / stale / failed) so
    /// operators see why a plugin is disabled without opening logs.
    @ViewBuilder
    private func attestBadge(_ state: String) -> some View {
        switch state.lowercased() {
        case "good", "ok":
            Text("verified").font(.system(size: 10)).foregroundColor(.green)
        case "stale":
            Text("stale").font(.system(size: 10)).foregroundColor(.orange)
        case "failed":
            Text("failed").font(.system(size: 10)).foregroundColor(.red)
        default:
            EmptyView()
        }
    }

    private func runInstall() {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = false
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.title = "Choose .leahplugin bundle"
        if panel.runModal() == .OK, let url = panel.url {
            actions.install(url)
            rows = actions.list()
        }
    }
}

