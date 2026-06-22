import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class WidgetTileRegistryTests: XCTestCase {
  private func env(_ widget: WidgetKind, id: String = "abc") -> WidgetEnvelope {
    WidgetEnvelope(widget: widget.rawValue, id: id, size: "small", refresh: nil, actions: nil, props: [:])
  }

  func testRegistryReturnsNilForUnregisteredKind() {
    let r = WidgetTileRegistry()
    XCTAssertNil(r.view(for: env(.market)))
  }

  func testRegistryReturnsViewForRegisteredKind() {
    let r = WidgetTileRegistry()
    r.register(kind: .stat) { e in AnyView(Text(e.id)) }
    XCTAssertNotNil(r.view(for: env(.stat, id: "x")))
  }

  func testReRegisterReplacesBuilder() {
    let r = WidgetTileRegistry()
    var calls = 0
    r.register(kind: .stat) { _ in calls = 1; return AnyView(Text("a")) }
    r.register(kind: .stat) { _ in calls = 2; return AnyView(Text("b")) }
    _ = r.view(for: env(.stat))
    XCTAssertEqual(calls, 2)
  }
}
