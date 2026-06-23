import AppKit
import SwiftUI

@MainActor
public final class DashboardWindow: NSWindowController {
  private let resolver: DashboardTileResolver

  public convenience init(resolver: DashboardTileResolver = .placeholder) {
    let win = DashboardWindow.makeWindow()
    self.init(window: win, resolver: resolver)
    win.contentViewController = NSHostingController(rootView: DashboardView(resolver: resolver))
  }

  private init(window: NSWindow, resolver: DashboardTileResolver) {
    self.resolver = resolver
    super.init(window: window)
  }

  required init?(coder: NSCoder) {
    self.resolver = .placeholder
    super.init(coder: coder)
  }

  public static func makeWindow() -> NSWindow {
    let frame = NSScreen.main?.frame ?? NSRect(x: 0, y: 0, width: 1440, height: 900)
    let win = NSWindow(
      contentRect: frame,
      styleMask: [.borderless, .fullSizeContentView],
      backing: .buffered,
      defer: false
    )
    win.collectionBehavior = [.fullScreenPrimary, .canJoinAllSpaces]
    win.isReleasedWhenClosed = false
    win.level = .normal
    win.backgroundColor = .black
    return win
  }

  public func toggle() {
    if window?.isVisible == true {
      window?.orderOut(nil)
    } else {
      showWindow(nil)
      window?.makeKeyAndOrderFront(nil)
    }
  }
}
