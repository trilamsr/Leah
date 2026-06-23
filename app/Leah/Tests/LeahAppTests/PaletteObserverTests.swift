import XCTest
import SwiftUI
@testable import LeahApp

final class PaletteObserverTests: XCTestCase {
  func testPaletteObserverFiresOnAppearanceChange() {
    let observer = PaletteObserver(initial: .dark)
    var received: [Palette.Mode] = []
    let token = observer.subscribe { received.append($0.mode) }
    observer.apply(.light)
    observer.apply(.dark)
    observer.unsubscribe(token)
    XCTAssertEqual(received, [.light, .dark])
  }

  func testCrossfadeDuration240ms() {
    XCTAssertEqual(PaletteObserver.crossfadeDuration, 0.240, accuracy: 0.0001)
  }

  func testLightModeTokenContrastWCAG_AA() {
    let p = Palette.forMode(.light)
    XCTAssertGreaterThanOrEqual(contrastRatio(p.ivoryText, p.obsidian1), 4.5)
    XCTAssertGreaterThanOrEqual(contrastRatio(p.textMuted, p.obsidian1), 4.5)
    XCTAssertGreaterThanOrEqual(contrastRatio(p.goldPrimary, p.obsidian1), 4.5)
  }

  func testForbiddenColorDriftZeroZeroAA0C() {
    for mode in [Palette.Mode.dark, .light] {
      for hex in Palette.forMode(mode).allHex {
        XCTAssertNotEqual(hex.uppercased(), "#0A0A0C", "palette emitted forbidden #0A0A0C in mode \(mode)")
      }
    }
  }

  func testReducedMotionSkipsAnimation() {
    let observer = PaletteObserver(initial: .dark, reduceMotion: { true })
    XCTAssertEqual(observer.effectiveAnimationDuration(), 0)
    let live = PaletteObserver(initial: .dark, reduceMotion: { false })
    XCTAssertEqual(live.effectiveAnimationDuration(), PaletteObserver.crossfadeDuration)
  }

  private func contrastRatio(_ a: Color, _ b: Color) -> Double {
    let la = relativeLuminance(a)
    let lb = relativeLuminance(b)
    let (hi, lo) = la > lb ? (la, lb) : (lb, la)
    return (hi + 0.05) / (lo + 0.05)
  }

  private func relativeLuminance(_ c: Color) -> Double {
    let ns = NSColor(c).usingColorSpace(.sRGB) ?? NSColor(c)
    let r = channel(Double(ns.redComponent))
    let g = channel(Double(ns.greenComponent))
    let b = channel(Double(ns.blueComponent))
    return 0.2126 * r + 0.7152 * g + 0.0722 * b
  }

  private func channel(_ v: Double) -> Double {
    return v <= 0.03928 ? v / 12.92 : pow((v + 0.055) / 1.055, 2.4)
  }
}
