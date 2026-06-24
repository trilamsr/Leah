import SwiftUI

/// Pane render state: loaded shows content, empty/error replaces with placeholder.
public enum PaneState {
    case loaded
    case empty
    case error(String)
}

public struct EmptyState: View {
    private let symbol: String
    private let title: String
    private let caption: String
    private let actionTitle: String?
    private let action: (() -> Void)?

    public init(symbol: String,
                title: String,
                caption: String,
                actionTitle: String? = nil,
                action: (() -> Void)? = nil) {
        self.symbol = symbol
        self.title = title
        self.caption = caption
        self.actionTitle = actionTitle
        self.action = action
    }

    public var body: some View {
        VStack(spacing: 12) {
            Image(systemName: symbol)
                .font(.system(size: 32, weight: .light))
                .foregroundColor(.gray)
            Text(title)
                .font(.system(size: 15, weight: .medium))
                .foregroundColor(.white)
            Text(caption)
                .font(.system(size: 12))
                .foregroundColor(.gray)
                .multilineTextAlignment(.center)
            if let actionTitle, let action {
                Button(actionTitle, action: action)
                    .padding(.top, 4)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding(32)
    }
}
