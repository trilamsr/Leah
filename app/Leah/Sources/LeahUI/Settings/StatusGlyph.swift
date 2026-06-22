import SwiftUI

public enum ConnectionState: Equatable {
    case connected
    case partial
    case none
}

public enum StatusGlyph {
    public static func symbol(for state: ConnectionState) -> String {
        switch state {
        case .connected: return "●"
        case .partial:   return "△"
        case .none:      return "○"
        }
    }

    public static func color(for state: ConnectionState) -> Color {
        switch state {
        case .connected: return .green
        case .partial:   return .yellow
        case .none:      return .gray
        }
    }
}

public struct StatusGlyphView: View {
    public let state: ConnectionState

    public init(state: ConnectionState) { self.state = state }

    public var body: some View {
        Text(StatusGlyph.symbol(for: state))
            .foregroundColor(StatusGlyph.color(for: state))
    }
}
