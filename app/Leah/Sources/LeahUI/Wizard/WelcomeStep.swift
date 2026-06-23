import SwiftUI
import AVFoundation

// Step 1: Death-Note-L styling — bold extruded blackletter slab, regal sleek.
// Forward scale on appear ("coming forward" intent).
// Voice preview via system TTS (no mic perm needed) per spec §8.2.
public struct WelcomeStep: View {
  let onContinue: () -> Void
  private let synth = AVSpeechSynthesizer()
  @State private var lScale: CGFloat = 0.55
  @State private var lOpacity: Double = 0.0
  @State private var subOpacity: Double = 0.0

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public var body: some View {
    VStack(spacing: 24) {
      Spacer()
      // Death-Note-L styling — heavy blackletter slab. Layered text creates
      // the extruded weight that single-pass Old English Text MT loses at
      // SwiftUI's default tracking. Three-stack: shadow (depth) + gold halo
      // (regal accent) + ivory face (legibility against obsidian).
      ZStack {
        // Halo: champagne gold glow.
        Text("L")
          .font(.custom("Old English Text MT", size: 240))
          .fontWeight(.heavy)
          .foregroundColor(Color(red: 201/255, green: 169/255, blue: 97/255))
          .blur(radius: 16)
          .opacity(0.6)
        // Face: ivory, heavy serif weight, slight italic for forward lean.
        Text("L")
          .font(.custom("Old English Text MT", size: 240))
          .fontWeight(.heavy)
          .foregroundColor(Color(red: 240/255, green: 230/255, blue: 210/255))
          .overlay(
            Text("L")
              .font(.custom("Old English Text MT", size: 240))
              .fontWeight(.heavy)
              .foregroundColor(.clear)
              .shadow(color: Color(red: 201/255, green: 169/255, blue: 97/255), radius: 1, x: 1, y: 1)
              .shadow(color: Color(red: 201/255, green: 169/255, blue: 97/255), radius: 1, x: -1, y: -1)
          )
      }
      .scaleEffect(lScale)
      .opacity(lOpacity)
      Text("personal assistant")
        .font(.system(size: 16, weight: .light, design: .serif))
        .italic()
        .tracking(2.0)
        .foregroundColor(Color(red: 232/255, green: 220/255, blue: 196/255))
        .opacity(subOpacity)
      Spacer()
      HStack {
        Spacer()
        Button(action: onContinue) {
          Text("Begin")
            .font(.system(size: 13, weight: .medium, design: .serif))
            .tracking(1.5)
            .padding(.horizontal, 28)
            .padding(.vertical, 10)
        }
        .opacity(subOpacity)
        .keyboardShortcut(.defaultAction)
      }
      .padding(.bottom, 8)
    }
    .padding(48)
    .frame(maxWidth: .infinity, maxHeight: .infinity)
    .background(Color(red: 8/255, green: 9/255, blue: 12/255))
    .onAppear {
      withAnimation(.easeOut(duration: 1.1)) {
        lScale = 1.0
        lOpacity = 1.0
      }
      withAnimation(.easeOut(duration: 0.7).delay(0.9)) {
        subOpacity = 1.0
      }
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
