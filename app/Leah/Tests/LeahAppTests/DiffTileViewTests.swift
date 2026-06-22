import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class DiffTileViewTests: XCTestCase {
  private func envelope(unified: String, filename: String? = nil) -> WidgetEnvelope {
    var props: [String: AnyCodable] = ["unified": AnyCodable(unified)]
    if let filename { props["filename"] = AnyCodable(filename) }
    return WidgetEnvelope(widget: "diff", id: "diff-1", size: "medium", props: props)
  }

  func testRejectsMissingUnifiedDiff() {
    let env = WidgetEnvelope(widget: "diff", id: "diff-1", size: "medium", props: [:])
    XCTAssertThrowsError(try DiffTilePayload(envelope: env)) { err in
      guard case DiffTilePayload.Error.unifiedDiffRequired = err else {
        return XCTFail("expected unifiedDiffRequired, got \(err)")
      }
    }
  }

  func testParsesAddedLineAsGoldHairline() throws {
    let p = try DiffTilePayload(envelope: envelope(unified: "@@ -1,1 +1,1 @@\n+added\n"))
    let added = p.lines.first { $0.kind == .added }
    XCTAssertNotNil(added)
    XCTAssertEqual(added?.hairlineColor, DiffTilePayload.LineKind.added.hairlineColor)
    // Champagne gold from Palette
    XCTAssertEqual(DiffTilePayload.LineKind.added.hairlineColor, LeahPalette.champagneGold)
  }

  func testParsesRemovedLineAsRedAlertHairline() throws {
    // Spec §10.1 item 13 — removed gutter uses the red-alert brand token
    // (`Color.leahRedAlert`), distinct from oxblood used for line-bg fill.
    let p = try DiffTilePayload(envelope: envelope(unified: "@@ -1,1 +1,1 @@\n-removed\n"))
    let removed = p.lines.first { $0.kind == .removed }
    XCTAssertNotNil(removed)
    XCTAssertEqual(removed?.hairlineColor, DiffTilePayload.LineKind.removed.hairlineColor)
    XCTAssertEqual(DiffTilePayload.LineKind.removed.hairlineColor, LeahPalette.redAlert)
  }

  func testHunkHeaderTracking() {
    // Body sans small-caps tracking +0.04em (spec §10.1 diff item).
    // SwiftUI measures tracking in points; +0.04em at body sans size 13 ≈ 0.52pt.
    XCTAssertEqual(DiffTilePayload.hunkHeaderTracking, 0.52, accuracy: 0.001)
  }

  func testContextLineIsNeitherGoldNorOxblood() throws {
    let p = try DiffTilePayload(envelope: envelope(unified: "@@ -1,1 +1,1 @@\n context\n"))
    let ctx = p.lines.first { $0.kind == .context }
    XCTAssertNotNil(ctx)
    XCTAssertNil(ctx?.hairlineColor)
  }

  func testHunkHeaderClassified() throws {
    let p = try DiffTilePayload(envelope: envelope(unified: "@@ -12,4 +12,6 @@\n context\n"))
    let header = p.lines.first { $0.kind == .hunkHeader }
    XCTAssertNotNil(header)
    XCTAssertEqual(header?.text, "@@ -12,4 +12,6 @@")
  }

  func testMixedDiffPreservesOrder() throws {
    let unified = "@@ -1,3 +1,4 @@\n ctx\n-old\n+new\n+extra\n"
    let p = try DiffTilePayload(envelope: envelope(unified: unified))
    let kinds = p.lines.map { $0.kind }
    XCTAssertEqual(kinds, [.hunkHeader, .context, .removed, .added, .added])
  }
}
