import SwiftUI

public struct PlusButton: View {
  let action: () -> Void

  public init(action: @escaping () -> Void) {
    self.action = action
  }

  public var body: some View {
    Button(action: action) {
      Text("+")
        .font(.system(size: 18, weight: .semibold))
        .foregroundColor(.leahGold)
        .frame(width: 24, height: 24)
        .overlay(
          Circle().stroke(LeahPalette.hairline, lineWidth: 1)
        )
    }
    .buttonStyle(.plain)
    .accessibilityLabel("Open widget gallery")
    .keyboardShortcut("w", modifiers: [.command, .shift])
  }
}
