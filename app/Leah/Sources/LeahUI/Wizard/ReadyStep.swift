import SwiftUI

// Step 6: "You're ready" — big ⌥Space reminder (32 pt glyphs) + Settings link per spec §8.6.
public struct ReadyStep: View {
  let onContinue: () -> Void

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public var body: some View {
    VStack(alignment: .center, spacing: 24) {
      Spacer()
      Text("You're ready.")
        .font(.system(size: 28, weight: .medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      HStack(spacing: 8) {
        Text("⌥")
          .font(.system(size: 32, weight: .bold, design: .monospaced))
          .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
        Text("Space")
          .font(.system(size: 32, weight: .bold, design: .monospaced))
          .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      }
      Text("Press it anywhere to summon Leah.")
        .font(.system(size: 14))
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      Spacer()
      HStack {
        Button("Open Settings") {
          NotificationCenter.default.post(name: .leahOpenSettings, object: nil)
        }
        .foregroundColor(.secondary)
        Spacer()
        Button("Let's go", action: onContinue)
      }
    }
    .padding(48)
  }
}
