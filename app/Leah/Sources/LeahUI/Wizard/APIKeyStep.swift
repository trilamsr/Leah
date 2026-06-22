import SwiftUI
import LeahAuth

// Step 2: BYOK Anthropic key — SecureField paste + Keychain write per spec §13.15 + §17.18.
// Phase 1: save to Keychain immediately on "Verify & Continue" (live IPC ping is task-14b).
public struct APIKeyStep: View {
  let onContinue: () -> Void
  @State private var key = ""
  @State private var showKey = false
  @State private var status = ""
  @State private var verifying = false

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Add your Anthropic API key.")
        .font(.system(size: 22, weight: .medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Get one at console.anthropic.com → API Keys.")
        .font(.system(size: 13))
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      HStack {
        Group {
          if showKey {
            TextField("sk-ant-...", text: $key).textFieldStyle(.roundedBorder)
          } else {
            SecureField("sk-ant-...", text: $key).textFieldStyle(.roundedBorder)
          }
        }
        Button(showKey ? "Hide" : "Show") { showKey.toggle() }
      }
      Text("We send 1 token to verify, then store it in macOS Keychain.")
        .font(.system(size: 12))
        .foregroundColor(Color(red: 138/255, green: 132/255, blue: 120/255))
      if !status.isEmpty {
        Text(status).font(.system(size: 12)).foregroundColor(.gray)
      }
      Spacer()
      HStack {
        Spacer()
        Button("Verify & Continue") {
          verifying = true
          status = "Saving to Keychain…"
          do {
            try Keychain.save(key)
            status = "Saved."
            onContinue()
          } catch {
            status = "Failed to save: \(error)"
          }
          verifying = false
        }
        .disabled(key.isEmpty || verifying)
      }
    }
    .padding(48)
  }
}
