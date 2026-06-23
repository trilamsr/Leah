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
    // SwiftUI App with only Settings scene runs as .accessory — wizard window
    // would orphan with no focus. Flip to .regular for the wizard's lifetime,
    // restore on close. Activate so window appears front.
    NSApp.setActivationPolicy(.regular)
    NSApp.activate(ignoringOtherApps: true)
    let view = WizardView(controller: self)
    let host = NSHostingController(rootView: view)
    let w = NSWindow(contentViewController: host)
    w.setContentSize(NSSize(width: 720, height: 520))
    w.styleMask = [.titled, .closable]
    w.title = "Leah"
    w.center()
    w.makeKeyAndOrderFront(nil)
    w.orderFrontRegardless()
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
    case .ready:
      currentStep = .done
      finish()
    case .done:        break
    }
  }

  /// finish closes the wizard window, releases the reference, and flips the
  /// app back to .accessory mode (Info.plist contract — LSUIElement=false
  /// gives us focus for onboarding, accessory after).
  @MainActor
  public func finish() {
    window?.close()
    window = nil
    NSApp.setActivationPolicy(.accessory)
  }
}
