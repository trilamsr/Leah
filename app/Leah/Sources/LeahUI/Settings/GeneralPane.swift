import SwiftUI

public struct GeneralPane: View {
    @State private var launchAtLogin = false

    public init() {}

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Group {
                Text("Hotkey").font(.system(size: 13, weight: .medium)).foregroundColor(.gray)
                Text("⌥Space").font(.system(size: 16, design: .monospaced)).foregroundColor(.white)
            }
            Divider()
            Group {
                Text("Theme").font(.system(size: 13, weight: .medium)).foregroundColor(.gray)
                Text("Dark (Phase 2 adds Light)").font(.system(size: 13)).foregroundColor(.white)
            }
            Divider()
            Toggle("Launch at login", isOn: $launchAtLogin)
                .foregroundColor(.white)
            Spacer()
        }
        .padding(24)
    }
}
