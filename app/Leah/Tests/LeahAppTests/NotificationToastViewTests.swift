import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class NotificationToastViewTests: XCTestCase {
  func testSwipeRightDismisses() {
    var stack = NotificationToastStack()
    let t = Toast(id: "t1", kind: "info", title: "Hello", body: "world", actionLabel: nil, severity: .info, createdAt: Date())
    stack.push(t)
    XCTAssertEqual(stack.visible.count, 1)
    stack.acknowledge(id: "t1")
    XCTAssertEqual(stack.visible.count, 0)
  }

  func testClickExpandsToFocusPanel() {
    var captured: Toast?
    let t = Toast(id: "t2", kind: "info", title: "Click me", body: "details", actionLabel: nil, severity: .info, createdAt: Date())
    let view = NotificationToastView(toast: t, onAcknowledge: { _ in }, onExpand: { captured = $0 })
    view.expand()
    XCTAssertEqual(captured?.id, "t2")
  }

  func testRendersActionChipRow() {
    let withAction = Toast(id: "t3", kind: "info", title: "Brief", body: "ready", actionLabel: "Open", severity: .info, createdAt: Date())
    let withoutAction = Toast(id: "t4", kind: "info", title: "Brief", body: "ready", actionLabel: nil, severity: .info, createdAt: Date())
    XCTAssertEqual(NotificationToastView.height(for: withAction), 112)
    XCTAssertEqual(NotificationToastView.height(for: withoutAction), 64)
  }
}
