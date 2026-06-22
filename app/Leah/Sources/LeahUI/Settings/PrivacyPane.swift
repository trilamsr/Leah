import SwiftUI

public struct PrivacyPane: View {
    @State private var telemetry = false

    public init() {}

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Toggle(isOn: $telemetry) {
                Text("Send anonymous error reports + performance metrics. Never includes message content.")
                    .font(.system(size: 13))
                    .foregroundColor(.white)
            }
            Divider()
            Text("Embed locally (slower, private) — Phase 2")
                .font(.system(size: 13)).foregroundColor(.gray)
            Divider()
            Text("Conversation history kept: 24 hours (locked Phase 1)")
                .font(.system(size: 13)).foregroundColor(.white)
            Spacer()
        }
        .padding(24)
    }
}
