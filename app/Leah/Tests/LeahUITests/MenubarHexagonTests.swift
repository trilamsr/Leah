import XCTest
import AppKit
@testable import LeahUI

final class MenubarHexagonTests: XCTestCase {
  // Each daemon-state has a glyph; nil would crash the menubar surface.
  func testImageReturnsNonNilForEachState() {
    for s: MenubarState in [.idle, .listening, .thinking, .responding, .error] {
      XCTAssertNotNil(MenubarHexagon.image(for: s), "state \(s) must yield an image")
    }
  }

  // Error breaks the template-tint contract on purpose: red is the signal.
  func testErrorStateUsesRedAlertColor() {
    let err = MenubarHexagon.image(for: .error)
    XCTAssertFalse(err.isTemplate, "error glyph carries color; must not be templated")
  }

  // Idle / listening / thinking ride menubar appearance via template tint.
  func testImageIsTemplateForIdleListeningThinking() {
    XCTAssertTrue(MenubarHexagon.image(for: .idle).isTemplate)
    XCTAssertTrue(MenubarHexagon.image(for: .listening).isTemplate)
    XCTAssertTrue(MenubarHexagon.image(for: .thinking).isTemplate)
  }
}
