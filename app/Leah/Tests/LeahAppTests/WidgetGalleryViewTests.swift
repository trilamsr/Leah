import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class WidgetGalleryViewTests: XCTestCase {
  func testSixFixedCategories() {
    XCTAssertEqual(WidgetGalleryCategory.allCases.count, 6)
    XCTAssertEqual(
      WidgetGalleryCategory.allCases.map(\.displayName),
      ["Finance", "Travel", "Time", "Productivity", "Web", "Code"]
    )
  }

  func testGalleryHasThreeSpawnAffordances() {
    let model = WidgetGalleryModel()
    XCTAssertFalse(model.isVisible)

    model.openFromPlusButton()
    XCTAssertTrue(model.isVisible)
    XCTAssertEqual(model.lastAffordance, .plusButton)
    model.dismiss()

    model.openFromHotkey()
    XCTAssertTrue(model.isVisible)
    XCTAssertEqual(model.lastAffordance, .hotkey)
    model.dismiss()

    XCTAssertTrue(model.consumeSlashIfPresent("/widgets"))
    XCTAssertTrue(model.isVisible)
    XCTAssertEqual(model.lastAffordance, .slashCommand)
  }

  func testPreviewIsLiveSmallTileNotScreenshot() {
    let registry = WidgetTileRegistry()
    registry.registerWave2Defaults()
    let item = WidgetGalleryCatalog.items.first { $0.kind == .market }!
    let envelope = item.sampleEnvelope()
    XCTAssertEqual(envelope.size, WidgetSize.small.rawValue)
    XCTAssertNotNil(
      registry.view(for: envelope),
      "preview must resolve to a live tile from WidgetTileRegistry, not a static Image"
    )
  }

  func testFuzzySearch_FindsArxivWidget() {
    let model = WidgetGalleryModel()
    model.searchQuery = "AAPL"
    let results = model.filteredItems
    XCTAssertTrue(
      results.contains { $0.kind == .market },
      "fuzzy search must match across sample-data text (AAPL is market sample data)"
    )
  }

  func testEscDismisses() {
    let model = WidgetGalleryModel()
    model.openFromHotkey()
    XCTAssertTrue(model.isVisible)
    model.handleEscape()
    XCTAssertFalse(model.isVisible)
  }

  func testReTypingSlashDismisses() {
    let model = WidgetGalleryModel()
    model.openFromPlusButton()
    XCTAssertTrue(model.isVisible)
    XCTAssertTrue(model.consumeSlashIfPresent("/widgets"))
    XCTAssertFalse(model.isVisible, "re-typing /widgets while open dismisses")
  }
}
