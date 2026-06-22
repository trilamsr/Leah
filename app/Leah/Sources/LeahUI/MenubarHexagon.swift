import AppKit

public enum MenubarHexagon {
  /// Draws an 18×18 hexagon as a TEMPLATE image so macOS tints it per
  /// menubar appearance. Outlined = idle; filled = listening; filled +
  /// inner dot = error (spec §4.2 — shape carries meaning, not color).
  public static func image(filled: Bool, hasDot: Bool) -> NSImage {
    let size = NSSize(width: 18, height: 18)
    let img = NSImage(size: size, flipped: false) { rect in
      let path = NSBezierPath()
      let cx = rect.midX, cy = rect.midY, r: CGFloat = 8
      for i in 0..<6 {
        let a = (CGFloat(i) * .pi / 3) - .pi / 2
        let p = NSPoint(x: cx + r * cos(a), y: cy + r * sin(a))
        if i == 0 { path.move(to: p) } else { path.line(to: p) }
      }
      path.close()
      path.lineWidth = 1.5
      NSColor.black.setStroke()
      NSColor.black.setFill()
      if filled { path.fill() } else { path.stroke() }
      if hasDot {
        let dot = NSBezierPath(ovalIn: NSRect(x: cx - 1.5, y: cy - 1.5, width: 3, height: 3))
        // punched-out center; template tinting inverts to accent on dark menubar
        NSColor.white.setFill()
        dot.fill()
      }
      return true
    }
    img.isTemplate = true
    return img
  }
}
