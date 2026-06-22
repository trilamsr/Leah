import XCTest
@testable import LeahApp

@MainActor
final class NotificationToastStackTests: XCTestCase {
  private func mk(_ id: String, severity: Toast.Severity = .info, at: Date = Date()) -> Toast {
    Toast(id: id, kind: "info", title: id, body: "", actionLabel: nil, severity: severity, createdAt: at)
  }

  func testStackCapsAtTwoVisible() {
    var stack = NotificationToastStack()
    stack.push(mk("a"))
    stack.push(mk("b"))
    stack.push(mk("c"))
    XCTAssertEqual(stack.visible.count, 2)
    XCTAssertEqual(stack.visible.map(\.id), ["a", "b"])
  }

  func testThirdToastCollapsesIntoPlusN() {
    var stack = NotificationToastStack()
    stack.push(mk("a"))
    stack.push(mk("b"))
    stack.push(mk("c"))
    stack.push(mk("d"))
    XCTAssertEqual(stack.overflowCount, 2)
    XCTAssertTrue(stack.hasOverflow)
  }

  func testFadeoutSharesSingleTimer() {
    let stack = NotificationToastStack()
    stack.push(mk("a"))
    stack.push(mk("b"))
    XCTAssertEqual(stack.activeTimerCount, 1, "spec perf #35: one coalesced timer across N toasts")
  }

  func testPriorityRedPersistsBeyond8s() {
    let now = Date()
    var stack = NotificationToastStack(now: { now.addingTimeInterval(20) })
    stack.push(mk("info", severity: .info, at: now))
    stack.push(mk("red", severity: .redAlert, at: now))
    stack.sweepExpired(defaultLifetime: 8)
    XCTAssertEqual(stack.visible.map(\.id), ["red"], "red-alert persists past 8s; info expires")
  }
}
