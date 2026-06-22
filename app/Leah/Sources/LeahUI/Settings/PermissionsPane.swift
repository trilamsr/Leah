import SwiftUI
import AVFoundation
import EventKit

public struct PermissionsPane: View {
    @State private var micGranted = AVCaptureDevice.authorizationStatus(for: .audio) == .authorized
    @State private var calGranted = EKEventStore.authorizationStatus(for: .event) == .fullAccess

    public init() {}

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            permissionRow(
                label: "Microphone",
                granted: micGranted,
                urlString: "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone"
            )
            Divider()
            permissionRow(
                label: "Accessibility (for ⌥Space hotkey)",
                granted: false,
                urlString: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
            )
            Divider()
            permissionRow(
                label: "Calendar",
                granted: calGranted,
                urlString: "x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars"
            )
            Spacer()
        }
        .padding(24)
    }

    private func permissionRow(label: String, granted: Bool, urlString: String) -> some View {
        HStack {
            Text(granted ? "●" : "○")
                .foregroundColor(granted ? .green : .gray)
            Text(label).foregroundColor(.white)
            Spacer()
            Button("Open Settings") {
                if let url = URL(string: urlString) {
                    NSWorkspace.shared.open(url)
                }
            }
        }
    }
}
