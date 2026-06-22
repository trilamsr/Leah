import AppKit
import SwiftUI
import LeahIPC

public final class FocusPanelController: NSObject {
  private var panel: NSPanel?
  private let client: IPCClient

  public init(client: IPCClient) {
    self.client = client
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
      let view = FocusPanelView(client: client)
      p.contentView = NSHostingView(rootView: view)
      // Dismiss on Esc via window delegate.
      p.delegate = self
      panel = p
    }
    panel?.center()
    panel?.makeKeyAndOrderFront(nil)
  }

  @MainActor
  public func dismiss() {
    panel?.orderOut(nil)
  }
}

extension FocusPanelController: NSWindowDelegate {
  public func windowDidResignKey(_ notification: Notification) {
    // Click-outside dismissal: panel loses key when user clicks elsewhere.
    dismiss()
  }
}
