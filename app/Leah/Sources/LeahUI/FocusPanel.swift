import AppKit
import SwiftUI
import LeahIPC

public final class FocusPanelController: NSObject {
  private var panel: NSPanel?
  private let client: IPCClient?
  // Fired after every dismiss() — used by callers and tests to observe closure.
  private let onDismiss: (() -> Void)?

  public var isVisible: Bool { panel?.isVisible ?? false }

  public init(client: IPCClient?, onDismiss: (() -> Void)? = nil) {
    self.client = client
    self.onDismiss = onDismiss
  }

  /// Builds the focus-panel NSPanel per spec §4.3 (testable independently).
  public static func makePanel() -> NSPanel {
    let panel = NSPanel(
      contentRect: NSRect(x: 0, y: 0, width: 860, height: 480),
      styleMask: [.titled, .nonactivatingPanel],
      backing: .buffered,
      defer: false
    )
    panel.level = .modalPanel
    panel.becomesKeyOnlyIfNeeded = false
    panel.worksWhenModal = true
    panel.collectionBehavior = [.fullScreenAuxiliary, .moveToActiveSpace, .stationary]
    panel.isMovableByWindowBackground = true
    panel.titlebarAppearsTransparent = true
    panel.titleVisibility = .hidden
    panel.appearance = NSAppearance(named: .darkAqua)
    return panel
  }

  @MainActor
  public func summon() {
    if panel == nil {
      let p = FocusPanelController.makePanel()
      if let c = client {
        p.contentView = NSHostingView(rootView: FocusPanelView(client: c))
      }
      p.delegate = self
      panel = p
    }
    panel?.center()
    panel?.makeKeyAndOrderFront(nil)
  }

  @MainActor
  public func dismiss() {
    panel?.orderOut(nil)
    onDismiss?()
  }

  // Esc dismissal (spec §4.3): called by the delegate's keyDown or
  // windowShouldClose when the user presses Escape.
  @MainActor
  public func handleEscapeKey() {
    dismiss()
  }
}

extension FocusPanelController: NSWindowDelegate {
  public func windowDidResignKey(_ notification: Notification) {
    // Click-outside dismissal: panel loses key when user clicks elsewhere.
    dismiss()
  }

  public func windowShouldClose(_ sender: NSWindow) -> Bool {
    // Esc on an NSPanel fires windowShouldClose; treat it as dismiss.
    dismiss()
    return true
  }
}
