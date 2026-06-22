import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class CalendarTileViewTests: XCTestCase {
  private func loadFixture() throws -> WidgetEnvelope {
    let url = Bundle.module.url(forResource: "calendar-today", withExtension: "json", subdirectory: "Fixtures")
    let data = try Data(contentsOf: XCTUnwrap(url))
    return try JSONDecoder().decode(WidgetEnvelope.self, from: data)
  }

  func testFixtureDecodesIntoCalendarPayload() throws {
    let env = try loadFixture()
    let payload = try CalendarPayload(props: env.props)
    XCTAssertEqual(payload.events.count, 4)
    XCTAssertEqual(payload.range, "today")
  }

  func testNextThreeEventsAreSelected() throws {
    let env = try loadFixture()
    let payload = try CalendarPayload(props: env.props)
    let visible = CalendarTileView.nextEvents(payload: payload, max: 3)
    XCTAssertLessThanOrEqual(visible.count, 3)
    XCTAssertEqual(visible.map(\.title), ["Spec review", "1:1 jen", "Arch sync"])
  }

  func testCurrentEventGetsGoldHairline() throws {
    let env = try loadFixture()
    let payload = try CalendarPayload(props: env.props)
    let visible = CalendarTileView.nextEvents(payload: payload, max: 3)
    let goldCount = visible.filter { CalendarTileView.hasGoldHairline(for: $0) }.count
    XCTAssertEqual(goldCount, 1)
    XCTAssertTrue(CalendarTileView.hasGoldHairline(for: visible[0]))
  }

  func testGoldRegionCountStaysWithinCanvasBudget() throws {
    let env = try loadFixture()
    let payload = try CalendarPayload(props: env.props)
    let visible = CalendarTileView.nextEvents(payload: payload, max: 3)
    let goldCount = visible.filter { CalendarTileView.hasGoldHairline(for: $0) }.count
    XCTAssertLessThanOrEqual(goldCount, 3)
  }

  func testHairlineWidthIsOnePointFive() {
    XCTAssertEqual(CalendarTileView.goldHairlineWidth, 1.5, accuracy: 0.001)
  }

  func testNowTickFractionMidEvent() throws {
    let env = try loadFixture()
    let payload = try CalendarPayload(props: env.props)
    let frac = try XCTUnwrap(CalendarTileView.nowTickFraction(payload: payload))
    XCTAssertGreaterThanOrEqual(frac, 0.0)
    XCTAssertLessThanOrEqual(frac, 1.0)
  }

  func testBodyRenders() throws {
    let env = try loadFixture()
    let payload = try CalendarPayload(props: env.props)
    let view = CalendarTileView(payload: payload)
    _ = view.body
  }
}
