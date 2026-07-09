import SwiftUI
import LeahAuth

// Step 2: BYOK Anthropic key — SecureField paste + Keychain write + optional
// daemon verify-key ping (spec §13.15, §17.18, §14b). verifyFn is injected so
// tests run without a live socket; daemon-offline degrades to a warning row so
// first-launch never hangs on a deaf daemon.
public struct APIKeyStep: View {
  let onContinue: () -> Void
  let verifyFn: (String) async -> IPCKeyVerifyOutcome

  @State private var key = ""
  @State private var showKey = false
  @State private var status = ""
  @State private var verifying = false

  public init(
    onContinue: @escaping () -> Void,
    verifyFn: @escaping (String) async -> IPCKeyVerifyOutcome = IPCKeyVerifier.live
  ) {
    self.onContinue = onContinue
    self.verifyFn = verifyFn
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Add your API key.")
        .font(.title2.weight(.medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Get one at console.anthropic.com → API Keys.")
        .font(.callout)
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
      Text("Stored securely in macOS Keychain.")
        .font(.caption)
        .foregroundColor(Color(red: 138/255, green: 132/255, blue: 120/255))
      if !status.isEmpty {
        Text(status).font(.caption).foregroundColor(.gray)
      }
      Spacer()
      HStack {
        Spacer()
        Button(verifying ? "Verifying…" : "Save & Continue") {
          Task { await saveAndVerify(key) }
        }
        .disabled(key.isEmpty || verifying)
      }
    }
    .padding(48)
  }

  // Internal seam: drives save→verify→continue without rendering, for unit tests.
  @MainActor
  func saveAndVerify(_ k: String) async {
    do {
      try Keychain.save(k)
    } catch {
      status = "Failed to save: \(error)"
      return
    }
    verifying = true
    status = "Verifying key…"
    let outcome = await verifyFn(k)
    verifying = false
    switch outcome {
    case .success:
      status = ""
      onContinue()
    case .rejected(let reason):
      // Hard fail — Anthropic said no. Block continue so the user can fix the key.
      status = reason
    case .offline:
      // Daemon unreachable or timed out — degrade gracefully. Key is in
      // Keychain; real call validates on first use.
      status = "Daemon offline — key saved; will verify on first use."
      onContinue()
    }
  }
}
