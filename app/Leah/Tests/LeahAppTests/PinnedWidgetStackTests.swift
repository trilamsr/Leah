import XCTest
import SwiftUI
@testable import LeahApp
@testable import LeahUI

@MainActor
final class PinnedWidgetStackTests: XCTestCase {
  private func envelope(_ id: String) -> WidgetEnvelope {
    WidgetEnvelope(widget: "market", id: id, size: "S")
  }

  private func stack(count: Int, onTap: @escaping () -> Void = {}) -> PinnedWidgetStack {
    PinnedWidgetStack(
      pinnedWidgets: (0..<count).map { envelope("p\($0)") },
      tile: { _ in AnyView(EmptyView()) },
      onOverflowTap: onTap
    )
  }

  func testCapConstantIsTwo() {
    XCTAssertEqual(PinnedWidgetStackPolicy.maxPinnedInAmbient, 2)
  }

  func testRendersAtMost2PinnedTiles() {
    XCTAssertEqual(stack(count: 0).visible.count, 0)
    XCTAssertEqual(stack(count: 1).visible.count, 1)
    XCTAssertEqual(stack(count: 2).visible.count, 2)
    XCTAssertEqual(stack(count: 5).visible.count, 2)
  }

  func testVisiblePrefixPreservesOrder() {
    let s = stack(count: 5)
    XCTAssertEqual(s.visible.map(\.id), ["p0", "p1"])
  }

  func testShowsDisclosureWhenPinnedExceeds2() {
    XCTAssertFalse(stack(count: 2).hasOverflow)
    XCTAssertTrue(stack(count: 3).hasOverflow)
    XCTAssertEqual(stack(count: 3).overflowCount, 1)
    XCTAssertEqual(stack(count: 7).overflowCount, 5)
  }

  func testNoOverflowAtOrBelowCap() {
    XCTAssertEqual(stack(count: 0).overflowCount, 0)
    XCTAssertEqual(stack(count: 1).overflowCount, 0)
    XCTAssertEqual(stack(count: 2).overflowCount, 0)
  }

  func testDisclosureTapOpensDashboard() {
    let tapped = expectation(description: "overflow tap fires")
    let s = stack(count: 4, onTap: { tapped.fulfill() })
    s.onOverflowTap()
    wait(for: [tapped], timeout: 0.1)
  }

  func testDefaultOverflowTapPostsOpenDashboardNotification() {
    let posted = expectation(description: "leahOpenDashboard posted")
    let token = NotificationCenter.default.addObserver(
      forName: .leahOpenDashboard, object: nil, queue: nil
    ) { _ in posted.fulfill() }
    defer { NotificationCenter.default.removeObserver(token) }

    PinnedWidgetStack.defaultOverflowTap()
    wait(for: [posted], timeout: 0.1)
  }
}
