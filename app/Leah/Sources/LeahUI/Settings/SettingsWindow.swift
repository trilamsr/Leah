import AppKit
import SwiftUI

public final class SettingsWindowController {
    private var window: NSWindow?

    public init() {}

    @MainActor
    public func show() {
        if let w = window { w.makeKeyAndOrderFront(nil); return }
        let host = NSHostingController(rootView: SettingsView())
        let w = NSWindow(contentViewController: host)
        w.setContentSize(NSSize(width: 720, height: 480))
        w.styleMask = [.titled, .closable]
        w.title = "Leah Settings"
        w.center()
        w.makeKeyAndOrderFront(nil)
        window = w
    }
}

struct SettingsView: View {
    @State private var selected: SettingsPane = .general
    @State private var searchQuery = ""

    var visiblePanes: [SettingsPane] {
        guard !searchQuery.isEmpty else { return SettingsPane.allCases }
        return SettingsPane.allCases.filter {
            $0.title.lowercased().contains(searchQuery.lowercased())
        }
    }

    var body: some View {
        HStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 4) {
                SearchBox(query: $searchQuery)
                    .padding(.horizontal, 8)
                    .padding(.top, 8)
                    .padding(.bottom, 4)
                ForEach(visiblePanes, id: \.self) { pane in
                    Text(pane.title)
                        .font(.system(size: 13))
                        .foregroundColor(selected == pane ? .white : .gray)
                        .padding(.vertical, 6)
                        .padding(.horizontal, 12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(selected == pane ? Color.accentColor.opacity(0.25) : Color.clear)
                        .cornerRadius(4)
                        .onTapGesture { selected = pane }
                        .keyboardShortcut(KeyEquivalent(Character(pane.keyboardShortcut)), modifiers: .command)
                }
                Spacer()
            }
            .frame(width: 180)
            .background(Color(red: 14/255, green: 16/255, blue: 20/255))
            Divider()
            Group {
                switch selected {
                case .general:      GeneralPane()
                case .privacy:      PrivacyPane()
                case .permissions:  PermissionsPane()
                case .advanced:     AdvancedPane()
                case .integrations: IntegrationsPane()
                case .memory:       MemoryPane()
                case .about:        AboutPane()
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color(red: 8/255, green: 9/255, blue: 12/255))
        }
    }
}
