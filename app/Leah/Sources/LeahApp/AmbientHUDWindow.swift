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
  // Perf-review item #29: HUD + notification toasts share one NSPanel. Each
  // extra NSPanel pulls ~4-8MB for the CALayer + a11y trees. Budget enforces
  // a single allocation for the ambient surface family.
  public static let maxPanelAllocations: Int = 1

  private var panel: NSPanel?
  private let data: AmbientHUDData
  private let anchor: AmbientHUDAnchor

  public init(data: AmbientHUDData = .empty, anchor: AmbientHUDAnchor = AmbientHUDWindow.defaultAnchor) {
    self.data = data
    self.anchor = anchor
  }

  public var allocatedPanelCount: Int { panel == nil ? 0 : 1 }

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
    mount(overlay: { Color.clear })
  }

  // Transient overlays (notification toasts, etc.) compose into the HUD panel's
  // root view via ZStack so the OS allocates one window, not one-per-surface.
  @MainActor
  public func mount<Overlay: View>(overlay: () -> Overlay) {
    if panel != nil { return }
    let p = AmbientHUDWindow.makePanel()
    let composed = ZStack(alignment: .topTrailing) {
      AmbientHUDView(data: data)
      overlay()
    }
    p.contentView = NSHostingView(rootView: composed)
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
