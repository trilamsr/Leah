import SwiftUI
import AVFoundation
import EventKit
import ApplicationServices
import AppKit

public struct PermissionsPane: View {
    @State private var micGranted = false
    @State private var calGranted = false
    @State private var axGranted = false

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
                granted: axGranted,
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
        .onAppear { refresh() }
        .onReceive(NotificationCenter.default.publisher(for: NSApplication.didBecomeActiveNotification)) { _ in
            refresh()
        }
    }

    private func refresh() {
        micGranted = AVCaptureDevice.authorizationStatus(for: .audio) == .authorized
        calGranted = EKEventStore.authorizationStatus(for: .event) == .fullAccess
        axGranted = AXIsProcessTrusted()
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
