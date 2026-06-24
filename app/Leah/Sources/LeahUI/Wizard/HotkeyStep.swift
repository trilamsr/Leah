import SwiftUI
import AppKit

// Step 2: ⌥Space default summon + Accessibility permission per spec §8.3.
// Custom-binding live editor is in Settings → Hotkey; we just point there to
// keep the wizard's blast radius small (one decision per step).
public struct HotkeyStep: View {
  let onContinue: () -> Void

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Press ⌥Space anywhere to summon Leah.")
        .font(.title2.weight(.medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("""
        You'll need to grant Accessibility permission. \
        Open System Settings → Privacy & Security → Accessibility → enable Leah.
        """)
        .font(.callout)
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      Text("Prefer a different shortcut? Change it later in Settings → Hotkey.")
        .font(.system(size: 12))
        .foregroundColor(Color(red: 138/255, green: 132/255, blue: 120/255))
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
