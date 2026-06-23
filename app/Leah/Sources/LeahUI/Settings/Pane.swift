import Foundation

public enum SettingsPane: String, CaseIterable, Hashable {
    case general, privacy, permissions, advanced, voice, appearance, integrations, memory, recommendations, about

    public var title: String {
        switch self {
        case .general:         return "General"
        case .privacy:         return "Privacy"
        case .permissions:     return "Permissions"
        case .advanced:        return "Advanced"
        case .voice:           return "Voice"
        case .appearance:      return "Appearance"
        case .integrations:    return "Integrations"
        case .memory:          return "Memory"
        case .recommendations: return "Recommendations"
        case .about:           return "About"
        }
    }

    public var keyboardShortcut: String {
        switch self {
        case .general:         return "1"
        case .privacy:         return "2"
        case .permissions:     return "3"
        case .advanced:        return "4"
        case .voice:           return "5"
        case .appearance:      return "6"
        case .integrations:    return "7"
        case .memory:          return "8"
        case .recommendations: return "9"
        case .about:           return "0"
        }
    }
}
