import AppKit
import SwiftUI
import LeahAuth

public final class WizardController: ObservableObject {
  public enum Step: Equatable {
    case welcome, apiKey, hotkey, mic, integration, firstPrompt, ready, done
  }

  // Set after a successful walkthrough OR an explicit Skip. Read by
  // shouldPresent() so a skipped wizard does not re-present on next launch
  // even though the keychain stays empty (default-OFF safe state).
  public static let completedKey = "leah.wizard.completed"

  @Published public private(set) var currentStep: Step = .welcome
  private var window: NSWindow?

  public init() {}

  public func shouldPresent() -> Bool {
    if UserDefaults.standard.bool(forKey: Self.completedKey) { return false }
    return (try? Keychain.read()) == nil
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
    case .integration: currentStep = .firstPrompt
    case .firstPrompt: currentStep = .ready
    case .ready:
      currentStep = .done
      finish()
    case .done:        break
    }
  }

  /// skip jumps directly to .done without touching any opt-in setting; the
  /// daemon stays in its default-OFF state. Persists wizard_completed so we
  /// do not re-present on next launch.
  @MainActor
  public func skip() {
    currentStep = .done
    finish()
  }

  /// finish closes the wizard window, releases the reference, persists the
  /// completed flag, and flips the app back to .accessory mode (Info.plist
  /// contract — LSUIElement=false gives us focus for onboarding, accessory
  /// after). NSApp is nil under XCTest; guard it.
  @MainActor
  public func finish() {
    UserDefaults.standard.set(true, forKey: Self.completedKey)
    window?.close()
    window = nil
    NSApp?.setActivationPolicy(.accessory)
  }
}
