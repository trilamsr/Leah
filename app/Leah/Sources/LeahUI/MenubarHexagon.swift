import AppKit

/// Daemon state surfaced through the menubar glyph. Five states match the
/// IPC event taxonomy (push-source pipeline §4.2): shape signals the channel,
/// color only joins for error so it can break template tinting and alert.
public enum MenubarState: Sendable, Equatable {
  case idle
  case listening
  case thinking
  case responding
  case error
}

public enum MenubarHexagon {
  /// Composes `hexagon.fill` with a state-specific SF Symbol overlay.
  /// Template for idle/listening/thinking/responding (tints with menubar
  /// appearance); error is colored so red reads regardless of mode.
  public static func image(for state: MenubarState) -> NSImage {
    switch state {
    case .idle:
      return baseHexagon()
    case .listening:
      return compose(overlay: "circle.fill", template: true)
    case .thinking:
      return compose(overlay: "ellipsis", template: true)
    case .responding:
      return compose(overlay: "waveform", template: true)
    case .error:
      return compose(overlay: "exclamationmark.triangle.fill",
                     template: false,
                     tint: .systemRed)
    }
  }

  private static let pointSize: CGFloat = 16
  private static let overlaySize: CGFloat = 9
  private static let canvas = NSSize(width: 18, height: 18)

  private static func baseHexagon() -> NSImage {
    let cfg = NSImage.SymbolConfiguration(pointSize: pointSize, weight: .regular)
    let img = NSImage(systemSymbolName: "hexagon.fill", accessibilityDescription: "Leah idle")?
      .withSymbolConfiguration(cfg) ?? fallback()
    img.isTemplate = true
    return img
  }

  private static func compose(overlay: String, template: Bool, tint: NSColor? = nil) -> NSImage {
    let baseCfg = NSImage.SymbolConfiguration(pointSize: pointSize, weight: .regular)
    let overlayCfg = NSImage.SymbolConfiguration(pointSize: overlaySize, weight: .bold)
    let baseSym = NSImage(systemSymbolName: "hexagon.fill", accessibilityDescription: nil)?
      .withSymbolConfiguration(baseCfg) ?? fallback()
    let overlaySym = NSImage(systemSymbolName: overlay, accessibilityDescription: nil)?
      .withSymbolConfiguration(overlayCfg) ?? fallback()

    let composed = NSImage(size: canvas, flipped: false) { rect in
      // Bottom-right placement matches macOS badge convention.
      baseSym.draw(in: rect)
      let ow: CGFloat = overlaySize
      let overlayRect = NSRect(x: rect.maxX - ow, y: 0, width: ow, height: ow)
      if let tint = tint {
        guard let tinted = overlaySym.copy() as? NSImage else {
          overlaySym.draw(in: overlayRect)
          return true
        }
        tinted.lockFocus()
        tint.set()
        NSRect(origin: .zero, size: tinted.size).fill(using: .sourceIn)
        tinted.unlockFocus()
        tinted.draw(in: overlayRect)
      } else {
        overlaySym.draw(in: overlayRect)
      }
      return true
    }
    composed.isTemplate = template
    return composed
  }

  private static func fallback() -> NSImage {
    // SF Symbols ship on macOS 11+; this branch only fires if a symbol id
    // changes upstream. Empty image keeps the menubar slot alive.
    NSImage(size: canvas)
  }
}
