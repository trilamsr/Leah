import XCTest
import Carbon.HIToolbox
@testable import LeahUI

final class HotkeyManagerTests: XCTestCase {
  // Pick an exotic chord (⌃⌥⇧F19) unlikely to collide with the test host's
  // shortcuts; ⌥Space would race with the user's running Leah session.
  private let probeKeyCode: UInt32 = UInt32(kVK_F19)
  private let probeModifiers: UInt32 =
    UInt32(controlKey) | UInt32(optionKey) | UInt32(shiftKey)

  func testRegister_SuccessReturnsToken() throws {
    let mgr = HotkeyManager {}
    let token = try mgr.register(keyCode: probeKeyCode, modifiers: probeModifiers)
    XCTAssertNotNil(token, "register should return a token on a free chord")
  }

  func testRegister_DuplicateReturnsConflictError() throws {
    let first = HotkeyManager {}
    _ = try first.register(keyCode: probeKeyCode, modifiers: probeModifiers)

    let second = HotkeyManager {}
    XCTAssertThrowsError(
      try second.register(keyCode: probeKeyCode, modifiers: probeModifiers)
    ) { error in
      guard case HotkeyRegistrationError.conflict = error else {
        XCTFail("expected .conflict, got \(error)")
        return
      }
    }
  }

  func testIsConflict_ProbesWithoutPersisting() throws {
    let probe = HotkeyManager {}
    XCTAssertFalse(
      probe.isConflict(keyCode: probeKeyCode, modifiers: probeModifiers),
      "free chord should not register as conflict"
    )
    // Probe must release on return — a follow-up register must still succeed.
    let mgr = HotkeyManager {}
    XCTAssertNoThrow(
      try mgr.register(keyCode: probeKeyCode, modifiers: probeModifiers),
      "probe must not leave Carbon state behind"
    )
  }
}
