import SwiftUI
import LeahAuth
import LeahIPC

// Step 2: BYOK Anthropic key — SecureField paste + Keychain write + IPC daemon ping
// per spec §13.15, §17.18, §14b.
// verifyFn is injected so tests capture the IPC ping without a live socket;
// production wiring uses IPCKeyVerifier.live.
public struct APIKeyStep: View {
  let onContinue: () -> Void
  // Async verify: returns nil on success, error string on failure.
  // Daemon-offline must return nil (degrade gracefully — key is saved, verify on first request).
  let verifyFn: (String) async -> String?

  @State private var key = ""
  @State private var showKey = false
  @State private var status = ""
  @State private var verifying = false

  public init(
    onContinue: @escaping () -> Void,
    verifyFn: @escaping (String) async -> String? = IPCKeyVerifier.live
  ) {
    self.onContinue = onContinue
    self.verifyFn = verifyFn
  }

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
      Text("Stored securely in macOS Keychain.")
        .font(.system(size: 12))
        .foregroundColor(Color(red: 138/255, green: 132/255, blue: 120/255))
      if !status.isEmpty {
        Text(status).font(.system(size: 12)).foregroundColor(.gray)
      }
      Spacer()
      HStack {
        Spacer()
        Button(verifying ? "Verifying…" : "Save & Continue") {
          Task { await saveAndVerify() }
        }
        .disabled(key.isEmpty || verifying)
      }
    }
    .padding(48)
  }

  @MainActor
  private func saveAndVerify() async {
    await saveAndVerifyKey(key)
  }

  // Internal seam for unit tests: drives the full save→verify→continue path
  // with a caller-supplied key without needing to render the SwiftUI body.
  @MainActor
  func testSaveAndVerify(key: String) async {
    await saveAndVerifyKey(key)
  }

  @MainActor
  private func saveAndVerifyKey(_ k: String) async {
    do {
      try Keychain.save(k)
    } catch {
      status = "Failed to save: \(error)"
      return
    }
    verifying = true
    status = "Verifying key…"
    if let errMsg = await verifyFn(k) {
      status = errMsg
      verifying = false
      return
    }
    verifying = false
    status = ""
    onContinue()
  }
}
