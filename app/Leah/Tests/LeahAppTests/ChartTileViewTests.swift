import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class ChartTileViewTests: XCTestCase {
  func testChartGoldOnlyOnAccentSeries() throws {
    let url = try XCTUnwrap(Bundle.module.url(forResource: "chart-line", withExtension: "json", subdirectory: "Fixtures"))
    let data = try Data(contentsOf: url)
    let env = try JSONDecoder().decode(WidgetEnvelope.self, from: data)

    let view = ChartTileView(envelope: env).frame(width: 480, height: 240)
    let renderer = ImageRenderer(content: view)
    renderer.scale = 1.0
    let cg = try XCTUnwrap(renderer.cgImage)
    XCTAssertGreaterThan(cg.width, 0)

    let goldPixels = countGoldPixels(cg)
    XCTAssertGreaterThan(goldPixels, 0, "accent series should render in gold")

    let regions = floodFillRegions(cg)
    XCTAssertLessThanOrEqual(regions, 3, "canvas invariant: ≤3 gold flood-fill regions per render")
  }
}
