import Foundation

struct DemoPrompt: Identifiable, Equatable {
  let id: UUID
  let icon: String
  let title: String
  let prompt: String

  init(id: UUID = UUID(), icon: String, title: String, prompt: String) {
    self.id = id
    self.icon = icon
    self.title = title
    self.prompt = prompt
  }
}

enum DemoPromptLibrary {
  static let regular: [DemoPrompt] = [
    DemoPrompt(icon: "calendar", title: "What's today?", prompt: "What's on my calendar today?"),
    DemoPrompt(icon: "envelope.badge", title: "Last meeting", prompt: "Summarize my last meeting"),
    DemoPrompt(icon: "newspaper", title: "AI news", prompt: "Brief me on the latest AI news"),
    DemoPrompt(icon: "clock.arrow.circlepath", title: "Recap yesterday", prompt: "What did I work on yesterday?"),
    DemoPrompt(icon: "square.and.pencil", title: "Draft email", prompt: "Help me draft an email"),
    DemoPrompt(icon: "sun.max", title: "Weather", prompt: "What's the weather today?"),
  ]
}
