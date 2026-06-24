import SwiftUI

public struct GeneralPane: View {
    @State private var launchAtLogin = false
    @State private var paneState: PaneState
    // Probe once at view-init: re-registering on every redraw would thrash Carbon.
    @State private var hotkeyConflict: Bool = GeneralPane.probeHotkeyConflict()

    public init(initialState: PaneState = .loaded) {
        _paneState = State(initialValue: initialState)
    }

    public var body: some View {
        switch paneState {
        case .empty:
            EmptyState(symbol: "slider.horizontal.3",
                       title: "No general settings yet",
                       caption: "Defaults will appear here once Leah finishes setup.")
        case .error(let msg):
            ErrorState(message: msg) { paneState = .loaded }
        case .loaded:
            loaded
        }
    }

    private var loaded: some View {
        VStack(alignment: .leading, spacing: 16) {
            Group {
                Text("Hotkey").font(.system(size: 13, weight: .medium)).foregroundColor(.gray)
                HStack(spacing: 8) {
                    Text("⌥Space").font(.system(size: 16, design: .monospaced)).foregroundColor(.white)
                    if hotkeyConflict {
                        Image(systemName: "exclamationmark.triangle")
                            .foregroundColor(.yellow)
                            .help("⌥Space is taken by another app. Free it in System Settings → Keyboard → Shortcuts.")
                    }
                }
            }
            if hotkeyConflict {
                HotkeyConflictWarning()
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

    private static func probeHotkeyConflict() -> Bool {
        HotkeyManager({}).isConflict(
            keyCode: HotkeyManager.optionSpaceKeyCode,
            modifiers: HotkeyManager.optionFlag
        )
    }
}
