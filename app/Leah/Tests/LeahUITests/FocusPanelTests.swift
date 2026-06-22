import XCTest
import AppKit
@testable import LeahUI

final class FocusPanelTests: XCTestCase {
  // Esc must dismiss the panel (spec §4.3).
  @MainActor
  func testEscKeyDismissesPanel() {
    var dismissed = false
    let controller = FocusPanelController(client: nil, onDismiss: { dismissed = true })
    controller.summon()
    XCTAssertTrue(controller.isVisible, "panel must be visible after summon()")
    controller.handleEscapeKey()
    XCTAssertTrue(dismissed, "onDismiss must fire on Esc")
    XCTAssertFalse(controller.isVisible, "panel must be hidden after Esc")
  }

  // Esc triggers BOTH windowShouldClose AND windowDidResignKey on the same
  // gesture — onDismiss must fire exactly once per summon→dismiss cycle.
  @MainActor
  func testDismissIsIdempotentWithinCycle() {
    var count = 0
    let controller = FocusPanelController(client: nil, onDismiss: { count += 1 })
    controller.summon()
    controller.dismiss()
    controller.dismiss()
    controller.dismiss()
    XCTAssertEqual(count, 1, "onDismiss must fire exactly once per summon")
    controller.summon()
    controller.dismiss()
    XCTAssertEqual(count, 2, "onDismiss must fire again after re-summon")
  }

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
