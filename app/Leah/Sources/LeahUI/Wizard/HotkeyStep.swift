import SwiftUI
import AppKit

// Step 3: Hotkey confirm + Accessibility permission per spec §8.3.
public struct HotkeyStep: View {
  let onContinue: () -> Void

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Press ⌥Space anywhere to summon Leah.")
        .font(.system(size: 22, weight: .medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("""
        You'll need to grant Accessibility permission. \
        Open System Settings → Privacy & Security → Accessibility → enable Leah.
        """)
        .font(.system(size: 13))
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      Spacer()
      HStack {
        Button("Open System Settings") {
          NSWorkspace.shared.open(
            URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")!
          )
        }
        Spacer()
        Button("Done", action: onContinue)
      }
    }
    .padding(48)
  }
}
