import Foundation

public enum SettingsPane: String, CaseIterable, Hashable {
    case general, privacy, permissions, advanced

    public var title: String {
        switch self {
        case .general:     return "General"
        case .privacy:     return "Privacy"
        case .permissions: return "Permissions"
        case .advanced:    return "Advanced"
        }
    }

    public var keyboardShortcut: String {
        switch self {
        case .general:     return "1"
        case .privacy:     return "2"
        case .permissions: return "3"
        case .advanced:    return "4"
        }
    }
}
