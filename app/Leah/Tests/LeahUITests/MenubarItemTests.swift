import XCTest
import AppKit
@testable import LeahUI

final class MenubarItemTests: XCTestCase {
  func testIdleHexagonIsOutlinedTemplate() {
    let img = MenubarHexagon.image(filled: false, hasDot: false)
    XCTAssertTrue(img.isTemplate, "menubar images MUST be template per spec §4.2")
    XCTAssertEqual(img.size.width, 18, accuracy: 1)
    XCTAssertEqual(img.size.height, 18, accuracy: 1)
  }

  func testListeningHexagonIsFilledTemplate() {
    let img = MenubarHexagon.image(filled: true, hasDot: false)
    XCTAssertTrue(img.isTemplate)
  }

  func testErrorHexagonHasInnerDotTemplate() {
    let img = MenubarHexagon.image(filled: true, hasDot: true)
    XCTAssertTrue(img.isTemplate)
  }

  @MainActor
  func testMenubarItemDefaultsToIdle() {
    let m = MenubarItem(onClick: {})
    XCTAssertEqual(m.currentState, .idle)
  }
}
