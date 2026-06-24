import SwiftUI

struct DemoPromptChipView: View {
  let prompt: DemoPrompt
  let onTap: () -> Void

  var body: some View {
    Button(action: onTap) {
      HStack(spacing: 6) {
        Image(systemName: prompt.icon)
          .font(.system(size: 12, weight: .medium))
        Text(prompt.title)
          .font(.system(size: 12, weight: .medium))
          .lineLimit(1)
      }
      .padding(.horizontal, 10)
      .padding(.vertical, 6)
      .background(
        Capsule().fill(Color.primary.opacity(0.06))
      )
      .overlay(
        Capsule().stroke(Color.primary.opacity(0.1), lineWidth: 0.5)
      )
      .foregroundStyle(Color.primary.opacity(0.85))
    }
    .buttonStyle(.plain)
    .accessibilityLabel(Text(prompt.title))
    .accessibilityHint(Text(prompt.prompt))
  }
}
