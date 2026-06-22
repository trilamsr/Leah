import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class CodeTileViewTests: XCTestCase {
  private func envelope(lang: String?, source: String) -> WidgetEnvelope {
    var props: [String: AnyCodable] = ["source": AnyCodable(source)]
    if let lang { props["language"] = AnyCodable(lang) }
    return WidgetEnvelope(widget: "code", id: "snip-1", size: "medium", props: props)
  }

  func testRejectsMissingLanguage() {
    XCTAssertThrowsError(try CodeTilePayload(envelope: envelope(lang: nil, source: "x"))) { err in
      guard case CodeTilePayload.Error.languageRequired = err else {
        return XCTFail("expected languageRequired, got \(err)")
      }
    }
  }

  func testRejectsEmptyLanguage() {
    XCTAssertThrowsError(try CodeTilePayload(envelope: envelope(lang: "", source: "x"))) { err in
      guard case CodeTilePayload.Error.languageRequired = err else {
        return XCTFail("expected languageRequired, got \(err)")
      }
    }
  }

  func testAcceptsLanguageAndSource() throws {
    let p = try CodeTilePayload(envelope: envelope(lang: "go", source: "package main"))
    XCTAssertEqual(p.language, "go")
    XCTAssertEqual(p.source, "package main")
  }

  func testEnforces16KBCap() {
    let oversized = String(repeating: "a", count: 16 * 1024 + 1)
    XCTAssertThrowsError(try CodeTilePayload(envelope: envelope(lang: "go", source: oversized))) { err in
      guard case CodeTilePayload.Error.sourceTooLarge(let n) = err else {
        return XCTFail("expected sourceTooLarge, got \(err)")
      }
      XCTAssertEqual(n, 16 * 1024 + 1)
    }
  }

  func testAcceptsExactly16KB() throws {
    let max = String(repeating: "a", count: 16 * 1024)
    let p = try CodeTilePayload(envelope: envelope(lang: "go", source: max))
    XCTAssertEqual(p.source.count, 16 * 1024)
  }

  // Cap is UTF-8 bytes, not grapheme count — a 5000-char emoji string is
  // 20 000 bytes and must reject even though .count = 5000 < 16384.
  func testRejectsMultiByteSourceOverByteCap() {
    let multibyte = String(repeating: "🚀", count: 5000)
    XCTAssertThrowsError(try CodeTilePayload(envelope: envelope(lang: "go", source: multibyte))) { err in
      guard case CodeTilePayload.Error.sourceTooLarge(let n) = err else {
        return XCTFail("expected sourceTooLarge, got \(err)")
      }
      XCTAssertEqual(n, 20_000)
    }
  }

  // Gold-budget invariant (canvas §10.0): ≤3 gold regions per visible tile.
  // CodeTilePayload exposes a highlighter that classifies tokens; "keyword"
  // is the only gold class. The view caps gold to 3 per visible region.
  func testGoldRegionsCappedAtThreeAcrossSource() throws {
    let src = """
    package main
    import "fmt"
    func main() {
      var x int = 1
      return
    }
    """
    let p = try CodeTilePayload(envelope: envelope(lang: "go", source: src))
    let regions = p.highlightedRegions()
    let goldCount = regions.filter { $0.role == .keyword }.count
    XCTAssertLessThanOrEqual(goldCount, 3,
      "gold (keyword) regions must be capped at 3 per visible tile; got \(goldCount)")
  }

  func testKeywordRoleIsRecognized() throws {
    let p = try CodeTilePayload(envelope: envelope(lang: "go", source: "func"))
    let regions = p.highlightedRegions()
    XCTAssertTrue(regions.contains { $0.role == .keyword && $0.text == "func" })
  }

  func testIdentifierRoleForNonKeyword() throws {
    let p = try CodeTilePayload(envelope: envelope(lang: "go", source: "myVar"))
    let regions = p.highlightedRegions()
    XCTAssertTrue(regions.contains { $0.role == .identifier && $0.text == "myVar" })
  }

  func testRejectsUnsupportedLanguage() {
    // No language guessing — payload requires lang, but the highlighter
    // also rejects unknown languages (otherwise it would silently guess).
    XCTAssertThrowsError(try CodeTilePayload(envelope: envelope(lang: "xyzlang", source: "x"))) { err in
      guard case CodeTilePayload.Error.unsupportedLanguage(let l) = err else {
        return XCTFail("expected unsupportedLanguage, got \(err)")
      }
      XCTAssertEqual(l, "xyzlang")
    }
  }
}
