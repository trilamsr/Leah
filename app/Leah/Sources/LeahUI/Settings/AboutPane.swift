import SwiftUI

public enum AboutInfo {
    public static var version: String {
        (Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String) ?? "0.0.0"
    }

    public static var commitSHA: String {
        (Bundle.main.object(forInfoDictionaryKey: "LEAHCommitSHA") as? String) ?? "dev"
    }

    public static let licenseURL = "https://github.com/trilamsr/Leah/blob/main/LICENSE"
}

public struct AboutPane: View {
    private let onCheckForUpdates: () -> Void
    @ObservedObject private var verification: VerificationModel
    @State private var paneState: PaneState

    @MainActor
    public init(
        onCheckForUpdates: @escaping () -> Void = {},
        verification: VerificationModel? = nil,
        initialState: PaneState = .loaded
    ) {
        self.onCheckForUpdates = onCheckForUpdates
        self.verification = verification ?? VerificationModel()
        _paneState = State(initialValue: initialState)
    }

    public var body: some View {
        switch paneState {
        case .empty:
            EmptyState(symbol: "info.circle",
                       title: "Version info unavailable",
                       caption: "Build metadata appears once Leah finishes loading its bundle.")
        case .error(let msg):
            ErrorState(message: msg) { paneState = .loaded }
        case .loaded:
            loaded
        }
    }

    private var loaded: some View {
        VStack(alignment: .leading, spacing: 16) {
            row("Version", value: AboutInfo.version)
            Divider()
            row("Commit", value: AboutInfo.commitSHA)
            Divider()
            HStack {
                Text("Updates").foregroundColor(.gray).font(.callout)
                Spacer()
                Button("Check for updates") { onCheckForUpdates() }
            }
            Divider()
            HStack(alignment: .top) {
                Text("Verification").foregroundColor(.gray).font(.callout)
                Spacer()
                VerificationPanel(model: verification)
            }
            Divider()
            HStack {
                Text("License").foregroundColor(.gray).font(.callout)
                Spacer()
                Link("View on GitHub", destination: URL(string: AboutInfo.licenseURL)!)
                    .foregroundColor(.accentColor)
            }
            Spacer()
        }
        .padding(24)
    }

    private func row(_ label: String, value: String) -> some View {
        HStack {
            Text(label).foregroundColor(.gray).font(.callout)
            Spacer()
            Text(value)
                .font(.system(.callout, design: .monospaced))
                .foregroundColor(.white)
                .textSelection(.enabled)
        }
    }
}
