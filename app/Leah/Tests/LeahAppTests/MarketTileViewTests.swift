import XCTest
import SwiftUI
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers
@testable import LeahApp

func pngBytes(from cg: CGImage) -> Data? {
  let mut = NSMutableData()
  guard let dst = CGImageDestinationCreateWithData(mut, UTType.png.identifier as CFString, 1, nil) else { return nil }
  CGImageDestinationAddImage(dst, cg, nil)
  guard CGImageDestinationFinalize(dst) else { return nil }
  return mut as Data
}

private func rgbaBuffer(_ cg: CGImage) -> (px: [UInt8], w: Int, h: Int)? {
  let w = cg.width, h = cg.height
  guard w > 0, h > 0 else { return nil }
  let cs = CGColorSpaceCreateDeviceRGB()
  var px = [UInt8](repeating: 0, count: w * h * 4)
  guard let ctx = CGContext(data: &px, width: w, height: h, bitsPerComponent: 8, bytesPerRow: w * 4, space: cs, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else { return nil }
  ctx.draw(cg, in: CGRect(x: 0, y: 0, width: w, height: h))
  return (px, w, h)
}

// Champagne gold target #C9A961 (201/169/97); accept ±28 per-channel to absorb
// blur, anti-alias, and opacity reductions used in sparkline strokes.
private func isGold(_ r: UInt8, _ g: UInt8, _ b: UInt8, _ a: UInt8) -> Bool {
  guard a > 24 else { return false }
  let dr = abs(Int(r) - 201), dg = abs(Int(g) - 169), db = abs(Int(b) - 97)
  return dr < 32 && dg < 32 && db < 32 && r > g && g > b
}

func countGoldPixels(_ cg: CGImage) -> Int {
  guard let buf = rgbaBuffer(cg) else { return 0 }
  var n = 0
  for i in stride(from: 0, to: buf.px.count, by: 4) {
    if isGold(buf.px[i], buf.px[i+1], buf.px[i+2], buf.px[i+3]) { n += 1 }
  }
  return n
}

// 4-connected flood-fill across gold-pixel mask; ignores islands smaller than
// 6 pixels (anti-alias noise). Returns the number of distinct gold regions.
func floodFillRegions(_ cg: CGImage) -> Int {
  guard let buf = rgbaBuffer(cg) else { return 0 }
  let w = buf.w, h = buf.h
  var mask = [Bool](repeating: false, count: w * h)
  for i in 0..<(w*h) {
    let p = i * 4
    mask[i] = isGold(buf.px[p], buf.px[p+1], buf.px[p+2], buf.px[p+3])
  }
  var seen = [Bool](repeating: false, count: w * h)
  var regions = 0
  for y in 0..<h {
    for x in 0..<w {
      let s = y * w + x
      if !mask[s] || seen[s] { continue }
      var stack = [(x, y)]
      var size = 0
      while let (cx, cy) = stack.popLast() {
        if cx < 0 || cx >= w || cy < 0 || cy >= h { continue }
        let k = cy * w + cx
        if seen[k] || !mask[k] { continue }
        seen[k] = true
        size += 1
        stack.append((cx + 1, cy))
        stack.append((cx - 1, cy))
        stack.append((cx, cy + 1))
        stack.append((cx, cy - 1))
      }
      if size >= 6 { regions += 1 }
    }
  }
  return regions
}

@MainActor
final class MarketTileViewTests: XCTestCase {
  func testRendersPriceAndDelta() throws {
    let url = try XCTUnwrap(Bundle.module.url(forResource: "market-aapl", withExtension: "json", subdirectory: "Fixtures"))
    let data = try Data(contentsOf: url)
    let env = try JSONDecoder().decode(WidgetEnvelope.self, from: data)

    let view = MarketTileView(envelope: env).frame(width: 320, height: 120)
    let renderer = ImageRenderer(content: view)
    renderer.scale = 1.0
    let cg = try XCTUnwrap(renderer.cgImage)
    let png = try XCTUnwrap(pngBytes(from: cg))
    XCTAssertGreaterThan(png.count, 0)

    let goldPixels = countGoldPixels(cg)
    XCTAssertGreaterThan(goldPixels, 0, "expected gold pixels for positive delta accent")

    let regions = floodFillRegions(cg)
    XCTAssertLessThanOrEqual(regions, 3, "canvas invariant: ≤3 gold flood-fill regions per render")
  }
}
