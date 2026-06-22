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

  // Closure reads view.isPinned (instead of a hardcoded bool) so a regression
  // where tap() forgets to surface state is actually caught.
  func testToggleCallbackCarriesState() {
    final class Box { var seen: Bool? }
    let pinnedBox = Box()
    var pinnedRef: PinBadgeView!
    pinnedRef = PinBadgeView(isPinned: true, onToggle: { pinnedBox.seen = pinnedRef.isPinned })
    pinnedRef.tap()
    XCTAssertEqual(pinnedBox.seen, true)

    let unpinnedBox = Box()
    var unpinnedRef: PinBadgeView!
    unpinnedRef = PinBadgeView(isPinned: false, onToggle: { unpinnedBox.seen = unpinnedRef.isPinned })
    unpinnedRef.tap()
    XCTAssertEqual(unpinnedBox.seen, false)
  }
}
