import SwiftUI
import AVFoundation
import AppKit

// Step 3: Mic permission per spec §8.4. Wake-word toggle UNCHECKED by default (operator decision #2).
// Permission calls ONLY from button action (not init) — AppKit gotcha.
public struct MicStep: View {
  // perf review item #26: hard-warn copy names the measured cost so the operator
  // opts in knowingly. Local hotword sustains ~1-5% CPU continuous on M1.
  public static let wakeWordCaption =
    "Adds ~2-4% to daily battery drain. Can enable later in Settings → Voice."

  // Denied-state recovery — surfacing the exact System Settings path keeps the
  // user from having to dig through three nested panes.
  public static let deniedRecoveryCaption =
    "System Settings → Privacy & Security → Microphone → enable Leah."

  let onContinue: () -> Void
  @AppStorage("leah.voice.wakeWord") private var wakeWordEnabled = false
  @State private var authStatus: AVAuthorizationStatus = .notDetermined

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Allow microphone access.")
        .font(.title2.weight(.medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Leah needs the microphone to hear your voice input.")
        .font(.callout)
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      switch authStatus {
      case .authorized:
        Label("Microphone access granted.", systemImage: "waveform")
          .font(.callout)
          .foregroundColor(.green)
      case .denied, .restricted:
        VStack(alignment: .leading, spacing: 6) {
          Label("Microphone access denied.", systemImage: "waveform.slash")
            .font(.system(size: 13))
            .foregroundColor(.red)
          Text(Self.deniedRecoveryCaption)
            .font(.system(size: 12))
            .foregroundColor(Color(red: 138/255, green: 132/255, blue: 120/255))
          Button("Open Privacy Settings") {
            NSWorkspace.shared.open(
              URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone")!
            )
          }
          .controlSize(.small)
        }
      default:
        Label("Microphone access not yet granted.", systemImage: "waveform.slash")
          .font(.callout)
          .foregroundColor(.gray)
      }
      Toggle(isOn: $wakeWordEnabled) {
        VStack(alignment: .leading, spacing: 2) {
          Text("Enable always-listening wake word")
            .font(.callout)
            .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
          Text(Self.wakeWordCaption)
            .font(.caption)
            .foregroundColor(Color(red: 138/255, green: 132/255, blue: 120/255))
        }
      }
      .padding(.top, 8)
      .accessibilityLabel("Enable always-listening wake word")
      .accessibilityHint(Self.wakeWordCaption)
      Spacer()
      HStack {
        Button("Allow Microphone") {
          // Request only from button action — AppKit requires user gesture context.
          AVCaptureDevice.requestAccess(for: .audio) { _ in
            DispatchQueue.main.async {
              authStatus = AVCaptureDevice.authorizationStatus(for: .audio)
            }
          }
        }
        .disabled(authStatus == .authorized)
        Spacer()
        Button("Continue", action: onContinue)
      }
    }
    .padding(48)
    .onAppear { authStatus = AVCaptureDevice.authorizationStatus(for: .audio) }
  }
}
