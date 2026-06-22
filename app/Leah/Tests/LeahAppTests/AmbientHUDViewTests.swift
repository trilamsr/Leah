import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class AmbientHUDViewTests: XCTestCase {
  private func clockAt(_ hour: Int, _ minute: Int = 0) -> () -> Date {
    var comps = DateComponents()
    comps.year = 2026; comps.month = 6; comps.day = 22
    comps.hour = hour; comps.minute = minute
    let cal = Calendar(identifier: .gregorian)
    let date = cal.date(from: comps)!
    return { date }
  }

  func testGreetingMorningStringFixed() {
    let view = AmbientHUDView(clock: clockAt(8), data: .empty)
    XCTAssertEqual(view.greeting(), "Good morning, Tri.")
  }

  func testGreetingAfternoonStringFixed() {
    let view = AmbientHUDView(clock: clockAt(13), data: .empty)
    XCTAssertEqual(view.greeting(), "Good afternoon, Tri.")
  }

  func testGreetingEveningStringFixed() {
    let view = AmbientHUDView(clock: clockAt(19), data: .empty)
    XCTAssertEqual(view.greeting(), "Good evening, Tri.")
    let lateNight = AmbientHUDView(clock: clockAt(2), data: .empty)
    XCTAssertEqual(lateNight.greeting(), "Good evening, Tri.")
  }

  func testNowRowAMShowsCalendarNextEvent() {
    let data = AmbientHUDData(
      nextEventTitle: "Standup in 12m",
      briefInProgressTitle: nil,
      firstUncompletedAgenda: nil,
      prCount: 0, briefCount: 0, arxivCount: 0
    )
    let view = AmbientHUDView(clock: clockAt(9), data: data)
    XCTAssertEqual(view.nowRowText(), "Standup in 12m")
  }

  func testNowRowPMShowsBriefTitle() {
    let data = AmbientHUDData(
      nextEventTitle: "should be ignored",
      briefInProgressTitle: "Wave 3 HUD chrome",
      firstUncompletedAgenda: "should be ignored",
      prCount: 0, briefCount: 0, arxivCount: 0
    )
    let view = AmbientHUDView(clock: clockAt(14), data: data)
    XCTAssertEqual(view.nowRowText(), "Wave 3 HUD chrome")
  }

  func testNowRowEveningShowsAgendaItem() {
    let data = AmbientHUDData(
      nextEventTitle: "ignored",
      briefInProgressTitle: "ignored",
      firstUncompletedAgenda: "review arxiv queue",
      prCount: 0, briefCount: 0, arxivCount: 0
    )
    let view = AmbientHUDView(clock: clockAt(20), data: data)
    XCTAssertEqual(view.nowRowText(), "review arxiv queue")
  }

  func testNowRowHiddenWhenNoData() {
    let view = AmbientHUDView(clock: clockAt(9), data: .empty)
    XCTAssertNil(view.nowRowText())
  }

  func testPulseRowDefaultPRsGlyph() {
    let data = AmbientHUDData(
      nextEventTitle: nil, briefInProgressTitle: nil, firstUncompletedAgenda: nil,
      prCount: 5, briefCount: 3, arxivCount: 12
    )
    let view = AmbientHUDView(clock: clockAt(9), data: data)
    let pulse = view.pulseText(rotation: 0)
    XCTAssertTrue(pulse.hasPrefix("\u{25C7}"), "expected default glyph ◇, got: \(pulse)")
    XCTAssertEqual(pulse, "\u{25C7} 5 PRs")
  }

  func testPulseRowRotatesOnHover() {
    let data = AmbientHUDData(
      nextEventTitle: nil, briefInProgressTitle: nil, firstUncompletedAgenda: nil,
      prCount: 5, briefCount: 3, arxivCount: 12
    )
    let view = AmbientHUDView(clock: clockAt(9), data: data)
    // index 0 → ◇ PRs, 1 → ⌬ briefs, 2 → ⎇ arxiv, then back to 0.
    XCTAssertEqual(view.pulseText(rotation: 0), "\u{25C7} 5 PRs")
    XCTAssertEqual(view.pulseText(rotation: 1), "\u{232C} 3 briefs")
    XCTAssertEqual(view.pulseText(rotation: 2), "\u{2387} 12 arxiv")
    XCTAssertEqual(view.pulseText(rotation: 3), "\u{25C7} 5 PRs")
  }

  func testHUDDimensions280x84() {
    XCTAssertEqual(AmbientHUDView.size.width, 280)
    XCTAssertEqual(AmbientHUDView.size.height, 84)
  }

  func testHUDNeverShowsTiempos() {
    // Visual-contract: the HUD never references the Tiempos serif family.
    // §3.3 reserves it for one dashboard moment only; HUD chrome is grotesk-only.
    XCTAssertFalse(AmbientHUDView.fontFamilies.contains { $0.localizedCaseInsensitiveContains("Tiempos") })
    XCTAssertFalse(AmbientHUDView.fontFamilies.contains { $0.localizedCaseInsensitiveContains("New York") })
  }

  func testGoldRegionsStayWithinCanvasBudget() {
    // §10 canvas invariant: ≤3 gold regions per surface.
    XCTAssertLessThanOrEqual(AmbientHUDView.goldRegionCount, 3)
  }

  func testWindowDefaultsToBottomRightAnchor() {
    XCTAssertEqual(AmbientHUDWindow.defaultAnchor, .bottomRight)
  }

  func testWindowFrameForBottomRightAnchor() {
    let screen = CGRect(x: 0, y: 0, width: 1440, height: 900)
    let frame = AmbientHUDWindow.frame(for: .bottomRight, on: screen, padding: 16)
    XCTAssertEqual(frame.width, 280)
    XCTAssertEqual(frame.height, 84)
    XCTAssertEqual(frame.maxX, 1440 - 16)
    XCTAssertEqual(frame.minY, 16)
  }
}
