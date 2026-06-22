import XCTest
@testable import LeahApp

final class WidgetEnvelopeTests: XCTestCase {
  private func env(
    widget: String = "stat",
    id: String = "abc",
    size: String = "small",
    refresh: Int? = nil,
    actions: [WidgetEnvelope.Action]? = nil,
    props: [String: AnyCodable] = [:]
  ) -> WidgetEnvelope {
    WidgetEnvelope(widget: widget, id: id, size: size, refresh: refresh, actions: actions, props: props)
  }

  func testEnvelopeRoundTrip() throws {
    let e = env(
      widget: "market",
      id: "aapl_1d",
      size: "medium",
      refresh: 60,
      actions: [.init(label: "Pin", callback: "leah://action/pin?id=aapl_1d", icon: nil)],
      props: ["symbol": AnyCodable("AAPL")]
    )
    let data = try JSONEncoder().encode(e)
    let decoded = try JSONDecoder().decode(WidgetEnvelope.self, from: data)
    XCTAssertEqual(decoded.widget, "market")
    XCTAssertEqual(decoded.id, "aapl_1d")
    XCTAssertEqual(decoded.size, "medium")
    XCTAssertEqual(decoded.refresh, 60)
    XCTAssertEqual(decoded.actions?.count, 1)
    XCTAssertEqual(decoded.actions?.first?.label, "Pin")
    XCTAssertEqual((decoded.props["symbol"]?.value as? String), "AAPL")
    XCTAssertNoThrow(try decoded.validate())
  }

  func testEnvelopeRejectsUnknownKind() {
    XCTAssertThrowsError(try env(widget: "bogus").validate())
  }

  func testEnvelopeRejectsUnknownSize() {
    XCTAssertThrowsError(try env(size: "huge").validate())
  }

  func testIDPattern() {
    XCTAssertNoThrow(try env(id: "a-b_9").validate())
    XCTAssertThrowsError(try env(id: "BAD").validate())
    XCTAssertThrowsError(try env(id: "").validate())
    XCTAssertThrowsError(try env(id: String(repeating: "a", count: 65)).validate())
    XCTAssertThrowsError(try env(id: "has space").validate())
  }

  func testRefreshMinimum() {
    XCTAssertNoThrow(try env(refresh: 5).validate())
    XCTAssertThrowsError(try env(refresh: 4).validate())
    XCTAssertNoThrow(try env(refresh: nil).validate())
  }

  func testActionMaxItems() {
    let five = (0..<5).map {
      WidgetEnvelope.Action(label: "L\($0)", callback: "leah://action/pin", icon: nil)
    }
    XCTAssertThrowsError(try env(actions: five).validate())
    XCTAssertNoThrow(try env(actions: Array(five.prefix(4))).validate())
  }

  func testCallbackPattern() {
    let good = WidgetEnvelope.Action(label: "Pin", callback: "leah://action/pin?id=x", icon: nil)
    let bad  = WidgetEnvelope.Action(label: "Pin", callback: "https://example.com", icon: nil)
    XCTAssertNoThrow(try env(actions: [good]).validate())
    XCTAssertThrowsError(try env(actions: [bad]).validate())
  }

  func testLabelMaxLen() {
    let tooLong = WidgetEnvelope.Action(
      label: String(repeating: "x", count: 33),
      callback: "leah://action/pin",
      icon: nil
    )
    XCTAssertThrowsError(try env(actions: [tooLong]).validate())
  }

  func testActionRequiresLabelAndCallback() {
    let empty = WidgetEnvelope.Action(label: "", callback: "leah://action/pin", icon: nil)
    XCTAssertThrowsError(try env(actions: [empty]).validate())
  }

  func testEnvelopeCap256KB() throws {
    let big = String(repeating: "x", count: 260 * 1024)
    let e = env(props: ["payload": AnyCodable(big)])
    XCTAssertThrowsError(try e.validate()) { err in
      XCTAssertTrue(String(describing: err).contains("256"))
    }
  }
}
