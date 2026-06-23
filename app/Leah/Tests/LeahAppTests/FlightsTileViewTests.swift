import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class FlightsTileViewTests: XCTestCase {
  private func loadFixture() throws -> FlightsPayload {
    let url = try XCTUnwrap(Bundle.module.url(forResource: "flights-sfo-lis", withExtension: "json", subdirectory: "Fixtures"))
    let data = try Data(contentsOf: url)
    return try JSONDecoder().decode(FlightsPayload.self, from: data)
  }

  func testDecodesFixturePayload() throws {
    let p = try loadFixture()
    XCTAssertEqual(p.origin, "SFO")
    XCTAssertEqual(p.destination, "LIS")
    XCTAssertEqual(p.cells.count, 8)
    XCTAssertEqual(p.cells.filter { $0.globalMin }.count, 1)
  }

  func testGlobalMinIdentifiedFromPayloadFlag() throws {
    let p = try loadFixture()
    let min = try XCTUnwrap(p.cells.first { $0.globalMin })
    XCTAssertEqual(min.priceCents, 68900)
    XCTAssertEqual(min.date, "2026-09-12")
  }

  func testStylingClassifiesCells() throws {
    let p = try loadFixture()
    let model = FlightsTileModel(payload: p)
    let styled = model.styledCells

    XCTAssertEqual(styled.filter { $0.style == .globalMin }.count, 1,
                   "exactly one global-min cell")
    XCTAssertEqual(styled.filter { $0.style == .rowMin }.count, 1,
                   "row-min count = 1 row (one row in fixture)")
    XCTAssertNotEqual(styled.first(where: { $0.style == .globalMin })?.cell.date,
                      styled.first(where: { $0.style == .rowMin })?.cell.date,
                      "row-min may not double-count the global-min cell")
  }

  func testHighFareTaggedOxblood() throws {
    let p = try loadFixture()
    let model = FlightsTileModel(payload: p)
    let above = model.styledCells.filter { $0.style == .high }
    XCTAssertFalse(above.isEmpty, "fixture has cells ≥30% above row median")
    for s in above {
      XCTAssertGreaterThanOrEqual(s.cell.priceCents, Int(Double(model.rowMedianCents(for: s.cell)) * 1.30))
    }
  }

  func testGoldRegionsAtMostThreePerRender() throws {
    let p = try loadFixture()
    let model = FlightsTileModel(payload: p)
    XCTAssertLessThanOrEqual(model.goldRegionCount, 3,
                             "canvas invariant §10.0 — gold ≤3 regions per render")
  }

  func testGoldBudgetCountsRouteLabelOnce() throws {
    let p = try loadFixture()
    let model = FlightsTileModel(payload: p)
    XCTAssertTrue(model.routeLabelUsesGold,
                  "origin→destination route label uses gold once (§10.1 #2)")
  }

  func testGlobalMinStyledAsFilledGoldWithObsidianText() throws {
    let p = try loadFixture()
    let model = FlightsTileModel(payload: p)
    let gm = try XCTUnwrap(model.styledCells.first { $0.style == .globalMin })
    XCTAssertEqual(gm.style.background, FlightsTileStyle.goldHex)
    XCTAssertEqual(gm.style.foreground, FlightsTileStyle.obsidianHex)
  }

  func testRowMinHairlineIsGoldOnePointFive() throws {
    let p = try loadFixture()
    let model = FlightsTileModel(payload: p)
    let rm = try XCTUnwrap(model.styledCells.first { $0.style == .rowMin })
    XCTAssertEqual(rm.style.leadingHairlineWidth, 1.5, accuracy: 0.0001)
    XCTAssertEqual(rm.style.leadingHairlineHex, FlightsTileStyle.goldHex)
  }

  func testInstantiateView() throws {
    let p = try loadFixture()
    _ = FlightsTileView(payload: p)
  }
}
