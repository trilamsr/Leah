import AppKit
import SwiftUI

public enum AmbientHUDAnchor: Equatable {
  case bottomRight
  case bottomLeft
  case topRight
  case topLeft
}

public final class AmbientHUDWindow: NSObject {
  public static let defaultAnchor: AmbientHUDAnchor = .bottomRight
  public static let padding: CGFloat = 16

  private var panel: NSPanel?
  private let data: AmbientHUDData
  private let anchor: AmbientHUDAnchor

  public init(data: AmbientHUDData = .empty, anchor: AmbientHUDAnchor = AmbientHUDWindow.defaultAnchor) {
    self.data = data
    self.anchor = anchor
  }

  public static func frame(for anchor: AmbientHUDAnchor, on screen: CGRect, padding: CGFloat) -> CGRect {
    let w = AmbientHUDView.size.width
    let h = AmbientHUDView.size.height
    switch anchor {
    case .bottomRight:
      return CGRect(x: screen.maxX - w - padding, y: screen.minY + padding, width: w, height: h)
    case .bottomLeft:
      return CGRect(x: screen.minX + padding, y: screen.minY + padding, width: w, height: h)
    case .topRight:
      return CGRect(x: screen.maxX - w - padding, y: screen.maxY - h - padding, width: w, height: h)
    case .topLeft:
      return CGRect(x: screen.minX + padding, y: screen.maxY - h - padding, width: w, height: h)
    }
  }

  public static func makePanel() -> NSPanel {
    let p = NSPanel(
      contentRect: NSRect(x: 0, y: 0, width: AmbientHUDView.size.width, height: AmbientHUDView.size.height),
      styleMask: [.borderless, .nonactivatingPanel],
      backing: .buffered,
      defer: false
    )
    // Floor level: above desktop, below modalPanel (focus panel sits at modalPanel).
    p.level = NSWindow.Level(rawValue: NSWindow.Level.modalPanel.rawValue - 1)
    p.collectionBehavior = [.fullScreenAuxiliary, .stationary, .moveToActiveSpace]
    p.isFloatingPanel = true
    p.becomesKeyOnlyIfNeeded = true
    p.hasShadow = true
    p.isOpaque = false
    p.backgroundColor = .clear
    p.appearance = NSAppearance(named: .darkAqua)
    return p
  }

  @MainActor
  public func mount() {
    if panel != nil { return }
    let p = AmbientHUDWindow.makePanel()
    p.contentView = NSHostingView(rootView: AmbientHUDView(data: data))
    if let screen = NSScreen.main?.visibleFrame {
      p.setFrame(AmbientHUDWindow.frame(for: anchor, on: screen, padding: AmbientHUDWindow.padding), display: true)
    }
    p.orderFront(nil)
    panel = p
  }

  @MainActor
  public func unmount() {
    panel?.orderOut(nil)
    panel = nil
  }
}
