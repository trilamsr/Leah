import XCTest
@testable import LeahUI
@testable import LeahAuth

final class WizardTests: XCTestCase {
  // APIKeyStep must invoke verifyFn with the saved key before calling onContinue.
  func testAPIKeyStepEmitsVerifyKey() async {
    var capturedKey: String?
    var continueCalled = false
    let step = APIKeyStep(
      onContinue: { continueCalled = true },
      verifyFn: { key in
        capturedKey = key
        return nil // simulate success
      }
    )
    // Drive the save+verify path directly via the internal async helper.
    await step.testSaveAndVerify(key: "sk-ant-test")
    XCTAssertEqual(capturedKey, "sk-ant-test", "verifyFn must receive the saved key")
    XCTAssertTrue(continueCalled, "onContinue must fire after successful verify")
    try? Keychain.delete()
  }

  // verifyFn returning an error must block onContinue.
  func testAPIKeyStepVerifyFailureBlocksContinue() async {
    var continueCalled = false
    let step = APIKeyStep(
      onContinue: { continueCalled = true },
      verifyFn: { _ in "Key rejected by Anthropic — check the key and try again." }
    )
    await step.testSaveAndVerify(key: "sk-bad")
    XCTAssertFalse(continueCalled, "onContinue must not fire when verify fails")
    try? Keychain.delete()
  }


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
