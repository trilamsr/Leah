import SwiftUI
import AVFoundation

// Step 4: Mic permission per spec §8.4. Wake-word toggle UNCHECKED by default (operator decision #2).
// Permission calls ONLY from button action (not init) — AppKit gotcha.
public struct MicStep: View {
  // perf review item #26: hard-warn copy names the measured cost so the operator
  // opts in knowingly. Local hotword sustains ~1-5% CPU continuous on M1.
  public static let wakeWordCaption =
    "Adds ~2-4% to daily battery drain. Can enable later in Settings → Voice."

  let onContinue: () -> Void
  @AppStorage("leah.voice.wakeWord") private var wakeWordEnabled = false
  @State private var granted = false

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Allow microphone access.")
        .font(.title2.weight(.medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Leah needs the microphone to hear your voice input.")
        .font(.callout)
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      if granted {
        Label("Microphone access granted.", systemImage: "waveform")
          .font(.callout)
          .foregroundColor(.green)
      } else {
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
          AVCaptureDevice.requestAccess(for: .audio) { ok in
            DispatchQueue.main.async { granted = ok }
          }
        }
        Spacer()
        Button("Continue", action: onContinue)
      }
    }
    .padding(48)
    .onAppear {
      granted = AVCaptureDevice.authorizationStatus(for: .audio) == .authorized
    }
  }
}
