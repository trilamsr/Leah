import SwiftUI

// Step 5: Pre-filled "Try Leah" prompt. Send → posts leahSummonFocusPanel so
// the HUD focus panel opens and streams a response, then advance the wizard.
// We do NOT touch FocusPanel itself; the demo prompt is editable here for
// users who want to type their own first question.
public struct FirstPromptStep: View {
  public static let defaultPrompt = "What can you do?"

  let onContinue: () -> Void
  let onSummon: () -> Void
  @State private var prompt: String = FirstPromptStep.defaultPrompt

  public init(
    onContinue: @escaping () -> Void,
    onSummon: @escaping () -> Void = { NotificationCenter.default.post(name: .leahSummonFocusPanel, object: nil) }
  ) {
    self.onContinue = onContinue
    self.onSummon = onSummon
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Try Leah.")
        .font(.system(size: 22, weight: .medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Send your first prompt. The focus panel opens and streams a response.")
        .font(.system(size: 13))
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      TextField("", text: $prompt, axis: .vertical)
        .textFieldStyle(.roundedBorder)
        .lineLimit(2...4)
        .font(.system(size: 14))
      Spacer()
      HStack {
        Button("Skip", action: onContinue)
          .buttonStyle(.plain)
          .foregroundColor(.secondary)
        Spacer()
        Button("Send") {
          onSummon()
          onContinue()
        }
        .disabled(prompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
        .keyboardShortcut(.defaultAction)
      }
    }
    .padding(48)
  }
}
