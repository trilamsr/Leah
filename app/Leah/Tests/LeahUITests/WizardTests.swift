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

  // advance() must mutate @Published state on MainActor — @MainActor annotation
  // on the method is the enforcement; this test exercises it from MainActor.
  @MainActor
  func testAdvanceRunsOnMainActor() {
    let c = WizardController()
    MainActor.assertIsolated()
    c.advance()
    XCTAssertEqual(c.currentStep, .apiKey)
  }

  // APIKeyStep must hand the saved key to verifyFn and call onContinue on success.
  @MainActor
  func testAPIKeyStepVerifySuccessContinues() async {
    defer { try? Keychain.delete() }
    var capturedKey: String?
    var continued = false
    let step = APIKeyStep(
      onContinue: { continued = true },
      verifyFn: { k in capturedKey = k; return .success }
    )
    await step.saveAndVerify("sk-ant-ok")
    XCTAssertEqual(capturedKey, "sk-ant-ok")
    XCTAssertTrue(continued)
  }

  // .rejected must block onContinue so the user can fix the key.
  @MainActor
  func testAPIKeyStepRejectedBlocksContinue() async {
    defer { try? Keychain.delete() }
    var continued = false
    let step = APIKeyStep(
      onContinue: { continued = true },
      verifyFn: { _ in .rejected(reason: "Key rejected.") }
    )
    await step.saveAndVerify("sk-ant-bad")
    XCTAssertFalse(continued)
  }

  // Daemon-offline / timeout must NOT block the wizard — degrade gracefully.
  @MainActor
  func testAPIKeyStepOfflineFallbackContinues() async {
    defer { try? Keychain.delete() }
    var continued = false
    let step = APIKeyStep(
      onContinue: { continued = true },
      verifyFn: { _ in .offline }
    )
    await step.saveAndVerify("sk-ant-anything")
    XCTAssertTrue(continued, "offline daemon must not block first-launch")
  }

  // Realistic offline path: no socket present → connect fails fast → offline.
  // Bounded under 1s so a regression that introduces an unbounded wait fails loudly.
  func testIPCKeyVerifierOfflineUnderTimeout() async {
    let missing = "/tmp/lh-missing-\(UUID().uuidString.prefix(8)).sock"
    let start = Date()
    let outcome = await IPCKeyVerifier.verify(key: "k", timeout: 0.5, socketPath: missing)
    let elapsed = Date().timeIntervalSince(start)
    XCTAssertEqual(outcome, .offline)
    XCTAssertLessThan(elapsed, 1.0, "missing-socket path must not block")
  }
}
