import SwiftUI

// PinBadgeView is the spec § 10.3 `◆` affordance on every tile top-right.
// Filled champagne diamond = pinned. Outline diamond = unpinned. Tapping
// toggles via the supplied callback — the caller wires that to the IPC
// pin-RPC (live tile) or `widget.dismiss` (ambient slot).
public struct PinBadgeView: View {
  public let isPinned: Bool
  public let onToggle: () -> Void

  public init(isPinned: Bool, onToggle: @escaping () -> Void) {
    self.isPinned = isPinned
    self.onToggle = onToggle
  }

  public func tap() { onToggle() }

  public var body: some View {
    Button(action: onToggle) {
      DiamondShape()
        .fill(isPinned ? LeahPalette.champagneGold : Color.clear)
        .overlay(DiamondShape().stroke(LeahPalette.champagneGold, lineWidth: 1))
        .frame(width: 10, height: 10)
        .accessibilityLabel(isPinned ? "Unpin widget" : "Pin widget")
    }
    .buttonStyle(.plain)
  }
}

struct DiamondShape: Shape {
  func path(in rect: CGRect) -> Path {
    var p = Path()
    p.move(to: CGPoint(x: rect.midX, y: rect.minY))
    p.addLine(to: CGPoint(x: rect.maxX, y: rect.midY))
    p.addLine(to: CGPoint(x: rect.midX, y: rect.maxY))
    p.addLine(to: CGPoint(x: rect.minX, y: rect.midY))
    p.closeSubpath()
    return p
  }
}
