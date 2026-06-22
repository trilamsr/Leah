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

  func testTransitionTable_spawningTransitions() {
    var m = WidgetLifecycle.Machine(size: .medium)
    XCTAssertFalse(m.apply(.update))
    XCTAssertEqual(m.state, .spawning)
    XCTAssertFalse(m.apply(.stale))
    XCTAssertEqual(m.state, .spawning)
    var e = WidgetLifecycle.Machine(size: .small)
    XCTAssertTrue(e.apply(.error))
    XCTAssertEqual(e.state, .error)
    var d = WidgetLifecycle.Machine(size: .small)
    XCTAssertTrue(d.apply(.dismiss))
    XCTAssertEqual(d.state, .dismissed)
  }

  func testTransitionTable_liveTransitions() {
    func live() -> WidgetLifecycle.Machine {
      var m = WidgetLifecycle.Machine(size: .medium)
      _ = m.apply(.mount)
      return m
    }
    var u = live()
    XCTAssertTrue(u.apply(.update))
    XCTAssertEqual(u.state, .refreshing)
    var s = live(); XCTAssertTrue(s.apply(.stale)); XCTAssertEqual(s.state, .stale)
    var e = live(); XCTAssertTrue(e.apply(.error)); XCTAssertEqual(e.state, .error)
    var d = live(); XCTAssertTrue(d.apply(.dismiss)); XCTAssertEqual(d.state, .dismissed)
  }

  func testTransitionTable_refreshingTransitions() {
    func refreshing() -> WidgetLifecycle.Machine {
      var m = WidgetLifecycle.Machine(size: .medium)
      _ = m.apply(.mount); _ = m.apply(.update)
      return m
    }
    var mt = refreshing(); XCTAssertFalse(mt.apply(.mount)); XCTAssertEqual(mt.state, .refreshing)
    var up = refreshing(); XCTAssertFalse(up.apply(.update)); XCTAssertEqual(up.state, .refreshing)
    var st = refreshing(); XCTAssertTrue(st.apply(.stale)); XCTAssertEqual(st.state, .stale)
    var er = refreshing(); XCTAssertTrue(er.apply(.error)); XCTAssertEqual(er.state, .error)
    var di = refreshing(); XCTAssertTrue(di.apply(.dismiss)); XCTAssertEqual(di.state, .dismissed)
  }

  func testTransitionTable_staleTransitions() {
    func stale() -> WidgetLifecycle.Machine {
      var m = WidgetLifecycle.Machine(size: .medium)
      _ = m.apply(.mount); _ = m.apply(.stale)
      return m
    }
    var mt = stale(); XCTAssertFalse(mt.apply(.mount)); XCTAssertEqual(mt.state, .stale)
    var up = stale(); XCTAssertFalse(up.apply(.update)); XCTAssertEqual(up.state, .stale)
    var st = stale(); XCTAssertTrue(st.apply(.stale)); XCTAssertEqual(st.state, .stale)
    var er = stale(); XCTAssertTrue(er.apply(.error)); XCTAssertEqual(er.state, .error)
    var di = stale(); XCTAssertTrue(di.apply(.dismiss)); XCTAssertEqual(di.state, .dismissed)
  }

  func testTransitionTable_errorTransitions() {
    func errored() -> WidgetLifecycle.Machine {
      var m = WidgetLifecycle.Machine(size: .medium)
      _ = m.apply(.mount); _ = m.apply(.error)
      return m
    }
    var mt = errored(); XCTAssertFalse(mt.apply(.mount)); XCTAssertEqual(mt.state, .error)
    var up = errored(); XCTAssertFalse(up.apply(.update)); XCTAssertEqual(up.state, .error)
    var st = errored(); XCTAssertTrue(st.apply(.stale)); XCTAssertEqual(st.state, .stale)
    var er = errored(); XCTAssertTrue(er.apply(.error)); XCTAssertEqual(er.state, .error)
    var di = errored(); XCTAssertTrue(di.apply(.dismiss)); XCTAssertEqual(di.state, .dismissed)
  }
}
