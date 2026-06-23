import AppKit
import SwiftUI

@MainActor
public final class DashboardWindow: NSWindowController, NSWindowDelegate {
  public static let defaultSize = NSSize(width: 1180, height: 760)
  public static let frameAutosaveName = "leah.dashboard.frame"

  public convenience init(resolver: DashboardTileResolver = .placeholder) {
    let win = DashboardWindow.makeWindow()
    self.init(window: win)
    win.contentViewController = NSHostingController(rootView: DashboardView(resolver: resolver))
    win.delegate = self
  }

  public override init(window: NSWindow?) {
    super.init(window: window)
  }

  required init?(coder: NSCoder) {
    super.init(coder: coder)
  }

  public static func makeWindow() -> NSWindow {
    let rect = NSRect(origin: .zero, size: defaultSize)
    let win = NSWindow(
      contentRect: rect,
      styleMask: [.titled, .closable, .resizable, .fullSizeContentView],
      backing: .buffered,
      defer: false
    )
    win.titlebarAppearsTransparent = true
    win.title = "Dashboard"
    win.isReleasedWhenClosed = false
    win.collectionBehavior = [.fullScreenPrimary]
    win.minSize = NSSize(width: 720, height: 480)
    if !win.setFrameUsingName(frameAutosaveName) {
      win.center()
    }
    win.setFrameAutosaveName(frameAutosaveName)
    return win
  }

  public func toggle() {
    if window?.isVisible == true {
      window?.performClose(nil)
    } else {
      showWindow(nil)
      NSApp?.activate(ignoringOtherApps: true)
    }
  }

  public override func keyDown(with event: NSEvent) {
    // Esc returns to ambient (spec §4.7 dismiss).
    if event.keyCode == 53 {
      window?.performClose(nil)
      return
    }
    super.keyDown(with: event)
  }
}
