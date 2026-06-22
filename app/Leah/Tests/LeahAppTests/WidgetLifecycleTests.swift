import XCTest
@testable import LeahApp

final class WidgetLifecycleTests: XCTestCase {
  func testTransitionTable_rejectsLiveToSpawning() {
    var sm = WidgetLifecycle.Machine(size: .medium)
    XCTAssertEqual(sm.state, .spawning)
    XCTAssertTrue(sm.apply(.mount))
    XCTAssertEqual(sm.state, .live)
    XCTAssertFalse(sm.apply(.mount))
    XCTAssertEqual(sm.state, .live)
    XCTAssertTrue(sm.apply(.dismiss))
    XCTAssertEqual(sm.state, .dismissed)
    XCTAssertFalse(sm.apply(.mount))
    XCTAssertFalse(sm.apply(.update))
    XCTAssertFalse(sm.apply(.stale))
    XCTAssertFalse(sm.apply(.error))
    XCTAssertEqual(sm.state, .dismissed)
  }

  func testReservesHeightOnSpawning() {
    let small = WidgetLifecycle.Machine(size: .small)
    let medium = WidgetLifecycle.Machine(size: .medium)
    let large = WidgetLifecycle.Machine(size: .large)
    XCTAssertEqual(small.reservedHeight, WidgetLifecycle.reservedHeight(for: .small))
    XCTAssertEqual(medium.reservedHeight, WidgetLifecycle.reservedHeight(for: .medium))
    XCTAssertEqual(large.reservedHeight, WidgetLifecycle.reservedHeight(for: .large))
    XCTAssertGreaterThan(small.reservedHeight, 0)
    XCTAssertGreaterThan(medium.reservedHeight, small.reservedHeight)
    XCTAssertGreaterThan(large.reservedHeight, medium.reservedHeight)
  }

  func testStaggersRevealBy80ms() {
    XCTAssertEqual(WidgetLifecycle.revealDelay(index: 0, reduceMotion: false), 0.0)
    XCTAssertEqual(WidgetLifecycle.revealDelay(index: 1, reduceMotion: false), 0.080, accuracy: 1e-9)
    XCTAssertEqual(WidgetLifecycle.revealDelay(index: 2, reduceMotion: false), 0.160, accuracy: 1e-9)
    XCTAssertEqual(WidgetLifecycle.revealDelay(index: 3, reduceMotion: false), 0.240, accuracy: 1e-9)
  }

  func testReducedMotionSkipsAnimation() {
    for i in 0...4 {
      XCTAssertEqual(WidgetLifecycle.revealDelay(index: i, reduceMotion: true), 0.0)
    }
    XCTAssertFalse(WidgetLifecycle.shouldAnimate(reduceMotion: true))
    XCTAssertTrue(WidgetLifecycle.shouldAnimate(reduceMotion: false))
  }
}
