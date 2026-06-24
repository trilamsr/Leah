import AppKit
import SwiftUI

public enum AppearanceTheme: String, CaseIterable, Hashable {
    case matchSystem, light, dark

    public var title: String {
        switch self {
        case .matchSystem: return "Match System"
        case .light:       return "Light"
        case .dark:        return "Dark"
        }
    }

    @MainActor
    var nsAppearance: NSAppearance? {
        switch self {
        case .matchSystem: return nil
        case .light:       return NSAppearance(named: .aqua)
        case .dark:        return NSAppearance(named: .darkAqua)
        }
    }
}

public struct AppearancePane: View {
    public static let themeKey = "leah.appearance.theme"
    public static let minimalModeKey = "leah.appearance.minimalMode"
    public static let accentBudgetText = "≤ 3 gold accents per render"

    public static func currentTheme() -> AppearanceTheme {
        guard let raw = UserDefaults.standard.string(forKey: themeKey),
              let theme = AppearanceTheme(rawValue: raw) else { return .matchSystem }
        return theme
    }

    @MainActor
    public static func setTheme(_ theme: AppearanceTheme) {
        UserDefaults.standard.set(theme.rawValue, forKey: themeKey)
        NSApp?.appearance = theme.nsAppearance
    }

    public static func minimalModeEnabled() -> Bool {
        UserDefaults.standard.bool(forKey: minimalModeKey)
    }

    public static func setMinimalModeEnabled(_ on: Bool) {
        UserDefaults.standard.set(on, forKey: minimalModeKey)
    }

    @State private var theme: AppearanceTheme = AppearancePane.currentTheme()
    @State private var minimalMode: Bool = AppearancePane.minimalModeEnabled()
    @State private var paneState: PaneState

    public init(initialState: PaneState = .loaded) {
        _paneState = State(initialValue: initialState)
    }

    public var body: some View {
        switch paneState {
        case .empty:
            EmptyState(symbol: "paintbrush",
                       title: "Appearance not configured",
                       caption: "Theme + minimal-mode controls appear once setup completes.")
        case .error(let msg):
            ErrorState(message: msg) { paneState = .loaded }
        case .loaded:
            loaded
        }
    }

    private var loaded: some View {
        VStack(alignment: .leading, spacing: 16) {
            Group {
                Text("Theme").font(.callout.weight(.medium)).foregroundColor(.gray)
                Picker("", selection: $theme) {
                    ForEach(AppearanceTheme.allCases, id: \.self) { Text($0.title).tag($0) }
                }
                .pickerStyle(.segmented)
                .onChange(of: theme) { _, newValue in Self.setTheme(newValue) }
            }
            Divider()
            Toggle("Minimal mode", isOn: $minimalMode)
                .foregroundColor(.white)
                .onChange(of: minimalMode) { _, newValue in Self.setMinimalModeEnabled(newValue) }
                .accessibilityHint("Hides decorative HUD elements and pulse animations.")
            Divider()
            HStack {
                Text("Accent budget").font(.callout.weight(.medium)).foregroundColor(.gray)
                Spacer()
                Text(Self.accentBudgetText).font(.callout).foregroundColor(.white)
            }
            Spacer()
        }
        .padding(24)
    }
}
