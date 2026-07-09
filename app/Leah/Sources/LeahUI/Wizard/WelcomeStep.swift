import SwiftUI
import AVFoundation
#if canImport(AppKit)
import AppKit
#endif

// Step 1: Death-Note-L styling — cursive script "L" (Snell Roundhand), the
// hairline-tail signature glyph Light Yagami's nemesis uses. Falls back to
// Apple Chancery if Snell missing; both ship in macOS Supplemental fonts so
// SwiftUI's font lookup always resolves to a calligraphic italic, never the
// system sans-serif. Forward scale on appear ("coming forward" intent).
// Voice preview via system TTS (no mic perm needed) per spec §8.2.
// Skip-wizard top-right routes to default-OFF setup; daemon never receives
// opt-in writes for the skipped run.
public struct WelcomeStep: View {
  let onContinue: () -> Void
  let onSkip: () -> Void
  private let synth = AVSpeechSynthesizer()
  @State private var lScale: CGFloat = 0.55
  @State private var lOpacity: Double = 0.0
  @State private var subOpacity: Double = 0.0

  // resolvedBrandFont picks the first installed calligraphic face. NSFont's
  // lookup returns the requested face only when it actually exists; checking
  // here keeps the heavy-glyph render path off the fallback to .systemFont,
  // which collapses the "L" to a thin vertical bar at display sizes.
  private static func resolvedBrandFont(size: CGFloat) -> Font {
    #if canImport(AppKit)
    // Helvetica is the brand mark per operator direction — clean modernist
    // sans-serif, no calligraphy. Bold weight reads as a logo at display
    // sizes; the guard against missing NSFont covers stripped-down test
    // environments only (Helvetica ships on every macOS).
    if NSFont(name: "Helvetica-Bold", size: size) != nil {
      return Font.custom("Helvetica-Bold", size: size)
    }
    #endif
    return .system(size: size, weight: .bold)
  }

  public init(onContinue: @escaping () -> Void, onSkip: @escaping () -> Void = {}) {
    self.onContinue = onContinue
    self.onSkip = onSkip
  }

  public var body: some View {
    ZStack(alignment: .topTrailing) {
      VStack(spacing: 24) {
        Spacer()
        // Layered cursive "L": gold halo (regal depth) + ivory face (legibility
        // against obsidian). Snell Roundhand's hairline tails read as the
        // Death Note "L" signature even at small render sizes.
        ZStack {
          // Halo: champagne gold glow.
          Text("L")
            .font(Self.resolvedBrandFont(size: 240))
            .foregroundColor(Color(red: 201/255, green: 169/255, blue: 97/255))
            .blur(radius: 16)
            .opacity(0.6)
          // Face: ivory cursive serif.
          Text("L")
            .font(Self.resolvedBrandFont(size: 240))
            .foregroundColor(Color(red: 240/255, green: 230/255, blue: 210/255))
            .shadow(color: Color(red: 201/255, green: 169/255, blue: 97/255).opacity(0.5), radius: 2, x: 1, y: 1)
        }
        .scaleEffect(lScale)
        .opacity(lOpacity)
        .accessibilityHidden(true)
        Text("personal chief-of-staff")
          .font(.system(.body, design: .serif).weight(.light))
          .italic()
          .tracking(2.0)
          .foregroundColor(Color(red: 232/255, green: 220/255, blue: 196/255))
          .opacity(subOpacity)
        Spacer()
        HStack {
          Spacer()
          Button(action: onContinue) {
            Text("Begin")
              .font(.system(.callout, design: .serif).weight(.medium))
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

      Button("Skip wizard", action: onSkip)
        .buttonStyle(.plain)
        .font(.system(size: 11, weight: .regular, design: .serif))
        .foregroundColor(Color(red: 138/255, green: 132/255, blue: 120/255))
        .padding(.top, 12)
        .padding(.trailing, 16)
        .opacity(subOpacity)
    }
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
