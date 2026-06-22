import AppKit
import SwiftUI
import LeahAuth

public final class WizardController: ObservableObject {
  public enum Step: Equatable {
    case welcome, apiKey, hotkey, mic, integration, ready, done
  }

  @Published public private(set) var currentStep: Step = .welcome
  private var window: NSWindow?

  public init() {}

  public func shouldPresent() -> Bool {
    (try? Keychain.read()) == nil
  }

  @MainActor
  public func presentIfNeeded() {
    guard shouldPresent() else { return }
    let view = WizardView(controller: self)
    let host = NSHostingController(rootView: view)
    let w = NSWindow(contentViewController: host)
    w.setContentSize(NSSize(width: 720, height: 520))
    w.styleMask = [.titled, .closable]
    w.title = "Welcome to Leah"
    w.center()
    w.makeKeyAndOrderFront(nil)
    window = w
  }

  @MainActor
  public func advance() {
    switch currentStep {
    case .welcome:     currentStep = .apiKey
    case .apiKey:      currentStep = .hotkey
    case .hotkey:      currentStep = .mic
    case .mic:         currentStep = .integration
    case .integration: currentStep = .ready
    case .ready:       currentStep = .done; window?.close()
    case .done:        break
    }
  }
}
