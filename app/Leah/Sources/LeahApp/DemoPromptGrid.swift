import SwiftUI

struct DemoPromptGrid: View {
  let prompts: [DemoPrompt]
  let onSelect: (DemoPrompt) -> Void

  init(prompts: [DemoPrompt] = DemoPromptLibrary.regular, onSelect: @escaping (DemoPrompt) -> Void) {
    self.prompts = prompts
    self.onSelect = onSelect
  }

  private let columns = [
    GridItem(.flexible(), spacing: 8),
    GridItem(.flexible(), spacing: 8),
  ]

  var body: some View {
    LazyVGrid(columns: columns, spacing: 8) {
      ForEach(prompts) { prompt in
        DemoPromptChipView(prompt: prompt) { onSelect(prompt) }
      }
    }
  }
}
