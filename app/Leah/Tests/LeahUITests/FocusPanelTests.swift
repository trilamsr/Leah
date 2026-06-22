import XCTest
import AppKit
@testable import LeahUI

final class FocusPanelTests: XCTestCase {
  @MainActor
  func testPanelHasNonActivatingStyleMask() {
    let panel = FocusPanelController.makePanel()
    XCTAssertTrue(panel.styleMask.contains(.nonactivatingPanel))
    XCTAssertTrue(panel.styleMask.contains(.titled))
    XCTAssertEqual(panel.level, .modalPanel)
    XCTAssertFalse(panel.becomesKeyOnlyIfNeeded)
    XCTAssertTrue(panel.worksWhenModal)
  }

  @MainActor
  func testPanelHasFocusPanelCollectionBehavior() {
    let panel = FocusPanelController.makePanel()
    let cb = panel.collectionBehavior
    XCTAssertTrue(cb.contains(.fullScreenAuxiliary))
    XCTAssertTrue(cb.contains(.moveToActiveSpace))
    XCTAssertTrue(cb.contains(.stationary))
    // Per spec §4.3: focus panel drops .canJoinAllSpaces (ambient HUD owns that).
    XCTAssertFalse(cb.contains(.canJoinAllSpaces))
  }

  @MainActor
  func testPanelDefaultSize() {
    let panel = FocusPanelController.makePanel()
    XCTAssertEqual(panel.contentRect(forFrameRect: panel.frame).width, 860, accuracy: 1)
    XCTAssertEqual(panel.contentRect(forFrameRect: panel.frame).height, 480, accuracy: 1)
  }
}
