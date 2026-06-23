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

    @MainActor
    public init(
        onCheckForUpdates: @escaping () -> Void = {},
        verification: VerificationModel? = nil
    ) {
        self.onCheckForUpdates = onCheckForUpdates
        self.verification = verification ?? VerificationModel()
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            row("Version", value: AboutInfo.version)
            Divider()
            row("Commit", value: AboutInfo.commitSHA)
            Divider()
            HStack {
                Text("Updates").foregroundColor(.gray).font(.system(size: 13))
                Spacer()
                Button("Check for updates") { onCheckForUpdates() }
            }
            Divider()
            HStack(alignment: .top) {
                Text("Verification").foregroundColor(.gray).font(.system(size: 13))
                Spacer()
                VerificationPanel(model: verification)
            }
            Divider()
            HStack {
                Text("License").foregroundColor(.gray).font(.system(size: 13))
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
            Text(label).foregroundColor(.gray).font(.system(size: 13))
            Spacer()
            Text(value)
                .font(.system(size: 13, design: .monospaced))
                .foregroundColor(.white)
                .textSelection(.enabled)
        }
    }
}
