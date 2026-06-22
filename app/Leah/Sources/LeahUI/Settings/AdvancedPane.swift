import SwiftUI

// Opus 4.8 escalation toggles per F4. Both default false.
// nextReply auto-resets after daemon consumes it; sessionWide persists until
// daemon restart or operator toggles off.
public struct AdvancedPane: View {
    @AppStorage("leah.opus.nextReply") private var nextReply = false
    @AppStorage("leah.opus.sessionWide") private var sessionWide = false

    public init() {}

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Group {
                Text("Model").font(.system(size: 13, weight: .medium)).foregroundColor(.gray)
                Text("Default: claude-sonnet-4-6").font(.system(size: 13)).foregroundColor(.white)
                Text("Router: claude-haiku-4-5").font(.system(size: 13)).foregroundColor(.white)
            }
            Divider()
            Toggle(isOn: $nextReply) {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Use Opus 4.8 for the next query only").foregroundColor(.white)
                    Text("Daemon model flips to claude-opus-4-8 for the next request, then auto-resets.")
                        .font(.system(size: 11)).foregroundColor(.gray)
                }
            }
            Toggle(isOn: $sessionWide) {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Use Opus 4.8 for this session").foregroundColor(.white)
                    Text("Persists until daemon restart or you toggle off.")
                        .font(.system(size: 11)).foregroundColor(.gray)
                }
            }
            Spacer()
        }
        .padding(24)
    }
}
