import SwiftUI
import AVFoundation

// Step 1: Leah voice preview via system TTS (no mic perm needed) per spec §8.2.
// Phase 2 swaps this for ElevenLabs Flash v2.5 streamed from daemon.
public struct WelcomeStep: View {
  let onContinue: () -> Void
  private let synth = AVSpeechSynthesizer()

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public var body: some View {
    VStack(alignment: .leading, spacing: 24) {
      Spacer()
      Text("Hi, I'm Leah.")
        .font(.system(size: 28, weight: .medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Your personal assistant.")
        .font(.system(size: 14))
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      Text("Six steps. Two minutes.")
        .font(.system(size: 12))
        .foregroundColor(Color(red: 138/255, green: 132/255, blue: 120/255))
      Spacer()
      HStack {
        Spacer()
        Button(action: onContinue) {
          Text("Begin")
            .font(.system(size: 14, weight: .medium))
            .padding(.horizontal, 24)
            .padding(.vertical, 8)
        }
      }
    }
    .padding(48)
    .onAppear {
      let phrase = "Hi, I'm Leah."
      let attributed = NSMutableAttributedString(string: phrase)
      let leahRange = (phrase as NSString).range(of: "Leah")
      attributed.addAttribute(
        NSAttributedString.Key(rawValue: AVSpeechSynthesisIPANotationAttribute),
        value: "ˈliːə",
        range: leahRange
      )
      let u = AVSpeechUtterance(attributedString: attributed)
      u.voice = AVSpeechSynthesisVoice(identifier: "com.apple.voice.premium.en-US.Ava")
        ?? AVSpeechSynthesisVoice(language: "en-US")
      u.volume = 0.5
      synth.speak(u)
    }
  }
}
