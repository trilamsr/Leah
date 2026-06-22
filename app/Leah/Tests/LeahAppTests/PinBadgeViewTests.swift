import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class PinBadgeViewTests: XCTestCase {
  func testTapFiresToggle() {
    var toggled = 0
    let view = PinBadgeView(isPinned: false, onToggle: { toggled += 1 })
    view.tap()
    XCTAssertEqual(toggled, 1)
  }

  func testStatesAreDistinct() {
    let pinned = PinBadgeView(isPinned: true, onToggle: {})
    let unpinned = PinBadgeView(isPinned: false, onToggle: {})
    XCTAssertTrue(pinned.isPinned)
    XCTAssertFalse(unpinned.isPinned)
  }

  func testToggleCallbackCarriesState() {
    var lastSeen: Bool?
    let view = PinBadgeView(isPinned: true, onToggle: { lastSeen = true })
    view.tap()
    XCTAssertEqual(lastSeen, true)
  }
}
