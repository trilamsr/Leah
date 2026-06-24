import SwiftUI

/// Embeddable conflict surface for the hotkey wizard step and Settings.
/// Lives in Wizard/ so HotkeyStep can compose it without sibling wizard work
/// stomping the file.
public struct HotkeyConflictWarning: View {
  public let chordLabel: String
  public let onOpenShortcuts: () -> Void

  public init(chordLabel: String = "⌥Space", onOpenShortcuts: @escaping () -> Void = HotkeyConflictWarning.openKeyboardShortcuts) {
    self.chordLabel = chordLabel
    self.onOpenShortcuts = onOpenShortcuts
  }

  public var body: some View {
    HStack(alignment: .top, spacing: 10) {
      Image(systemName: "exclamationmark.triangle.fill")
        .foregroundStyle(.yellow)
        .font(.system(size: 14))
      VStack(alignment: .leading, spacing: 4) {
        Text("\(chordLabel) is already taken by another app.")
          .font(.system(size: 13, weight: .medium))
          .foregroundColor(.primary)
        Text("Leah cannot summon until you free this shortcut or assign a new one.")
          .font(.system(size: 12))
          .foregroundColor(.secondary)
        Button("Open Keyboard Shortcuts", action: onOpenShortcuts)
          .font(.system(size: 12))
          .padding(.top, 2)
      }
      Spacer(minLength: 0)
    }
    .padding(12)
    .background(Color.yellow.opacity(0.08))
    .overlay(
      RoundedRectangle(cornerRadius: 8)
        .stroke(Color.yellow.opacity(0.4), lineWidth: 1)
    )
    .clipShape(RoundedRectangle(cornerRadius: 8))
  }

  public static func openKeyboardShortcuts() {
    if let url = URL(string: "x-apple.systempreferences:com.apple.preference.keyboard?Shortcuts") {
      NSWorkspace.shared.open(url)
    }
  }
}
