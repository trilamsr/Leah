import SwiftUI
import AVFoundation
import LeahAuth

// Step 6: Wrap-up. ⌥Space reminder + summary of what got enabled +
// Settings deeplink. Done → controller.finish() persists wizard_completed
// so the wizard never re-presents.
public struct ReadyStep: View {
  let onContinue: () -> Void

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public var body: some View {
    VStack(alignment: .center, spacing: 18) {
      Spacer()
      Text("You're ready.")
        .font(.title.weight(.medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      HStack(spacing: 8) {
        Text("⌥")
          .font(.system(.largeTitle, design: .monospaced).weight(.bold))
          .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
        Text("Space")
          .font(.system(.largeTitle, design: .monospaced).weight(.bold))
          .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      }
      Text("Press it anywhere to summon Leah.")
        .font(.body)
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      summaryBlock
      Spacer()
      HStack {
        Button("Open Settings") {
          NotificationCenter.default.post(name: .leahOpenSettings, object: nil)
        }
        .foregroundColor(.secondary)
        Spacer()
        Button("Done", action: onContinue)
          .keyboardShortcut(.defaultAction)
      }
    }
    .padding(48)
  }

  private var summaryBlock: some View {
    let keyOK = (try? Keychain.read()) != nil
    let micOK = AVCaptureDevice.authorizationStatus(for: .audio) == .authorized
    let wakeOn = UserDefaults.standard.bool(forKey: "leah.voice.wakeWord")
    return VStack(alignment: .leading, spacing: 4) {
      summaryRow("API key", on: keyOK)
      summaryRow("Microphone", on: micOK)
      summaryRow("Wake word", on: wakeOn)
    }
    .font(.system(size: 12))
    .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
    .padding(.top, 8)
  }

  private func summaryRow(_ name: String, on: Bool) -> some View {
    HStack(spacing: 8) {
      Text(on ? "●" : "○").foregroundColor(on ? .green : .gray)
      Text("\(name): \(on ? "enabled" : "off")")
    }
  }
}
