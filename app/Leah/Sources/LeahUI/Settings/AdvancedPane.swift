import SwiftUI

// Opus 4.8 escalation toggles per F4. Both default false.
// nextReply auto-resets after daemon consumes it; sessionWide persists until
// daemon restart or operator toggles off.
public struct AdvancedPane: View {
    @AppStorage("leah.opus.nextReply") private var nextReply = false
    @AppStorage("leah.opus.sessionWide") private var sessionWide = false
    @AppStorage(AdvancedPane.useRollbackChannelKey) private var useRollbackChannel = false

    // Pinned to LeahUpdate's UpdateChannelPolicy — flipping this toggle widens
    // Sparkle's allowed-channel set to include "rollback", which surfaces the
    // last-known-good build when a freshly notarized release regresses.
    public static let useRollbackChannelKey = "leah.update.useRollbackChannel"
    public static func useRollbackChannelEnabled() -> Bool {
        UserDefaults.standard.bool(forKey: useRollbackChannelKey)
    }

    @State private var paneState: PaneState

    public init(initialState: PaneState = .loaded) {
        _paneState = State(initialValue: initialState)
    }

    public var body: some View {
        switch paneState {
        case .empty:
            EmptyState(symbol: "wrench.and.screwdriver",
                       title: "Advanced options unavailable",
                       caption: "Model + update-channel toggles appear once the daemon is reachable.")
        case .error(let msg):
            ErrorState(message: msg) { paneState = .loaded }
        case .loaded:
            loaded
        }
    }

    private var loaded: some View {
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
            Divider()
            Toggle(isOn: $useRollbackChannel) {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Use rollback channel for updates").foregroundColor(.white)
                    Text("Surfaces the previous known-good build if the current release regressed.")
                        .font(.system(size: 11)).foregroundColor(.gray)
                }
            }
            Spacer()
        }
        .padding(24)
    }
}
