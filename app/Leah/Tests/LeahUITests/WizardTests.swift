import XCTest
@testable import LeahUI
@testable import LeahAuth

final class WizardTests: XCTestCase {
  @MainActor
  func testWizardSkipsWhenKeychainPopulated() throws {
    try Keychain.save("sk-ant-present")
    defer { try? Keychain.delete() }
    let c = WizardController()
    XCTAssertFalse(c.shouldPresent())
  }

  @MainActor
  func testWizardPresentsWhenKeychainEmpty() throws {
    try? Keychain.delete()
    let c = WizardController()
    XCTAssertTrue(c.shouldPresent())
  }

  @MainActor
  func testWizardWalksAllSixSteps() {
    let c = WizardController()
    XCTAssertEqual(c.currentStep, .welcome)
    c.advance(); XCTAssertEqual(c.currentStep, .apiKey)
    c.advance(); XCTAssertEqual(c.currentStep, .hotkey)
    c.advance(); XCTAssertEqual(c.currentStep, .mic)
    c.advance(); XCTAssertEqual(c.currentStep, .integration)
    c.advance(); XCTAssertEqual(c.currentStep, .ready)
    c.advance(); XCTAssertEqual(c.currentStep, .done)
  }
}
