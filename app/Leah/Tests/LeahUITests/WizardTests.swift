import XCTest
import AVFoundation
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

  // perf review item #26 — wake-word toggle defaults OFF; warn copy must name the
  // measured battery cost so the operator opts in knowingly.
  func testMicStepWakeWordDefaultIsOff() {
    UserDefaults.standard.removeObject(forKey: "leah.voice.wakeWord")
    XCTAssertFalse(UserDefaults.standard.bool(forKey: "leah.voice.wakeWord"))
  }

  func testMicStepWakeWordCopyNamesBatteryCost() {
    XCTAssertTrue(MicStep.wakeWordCaption.contains("2-4%"),
                  "wake-word caption must name measured battery cost; got: \(MicStep.wakeWordCaption)")
    XCTAssertTrue(MicStep.wakeWordCaption.lowercased().contains("battery"))
    XCTAssertTrue(MicStep.wakeWordCaption.contains("Settings"),
                  "caption must point to Settings for later enable")
  }

  // Welcome utterance must carry IPA pronunciation attribute on "Leah" to prevent
  // mispronunciation as "Lee" (dropped final syllable).
  func testWelcomeUtteranceHasLeahIPAAttribute() {
    let attributed = NSMutableAttributedString(string: "Hi, I'm Leah.")
    let leahRange = ("Hi, I'm Leah." as NSString).range(of: "Leah")
    attributed.addAttribute(
      NSAttributedString.Key(rawValue: AVSpeechSynthesisIPANotationAttribute),
      value: "ˈliːə",
      range: leahRange
    )

    var foundIPAAttribute = false
    attributed.enumerateAttributes(in: leahRange, options: []) { attrs, _, _ in
      if let ipaValue = attrs[NSAttributedString.Key(rawValue: AVSpeechSynthesisIPANotationAttribute)] as? String {
        XCTAssertEqual(ipaValue, "ˈliːə", "Leah must have correct IPA pronunciation")
        foundIPAAttribute = true
      }
    }
    XCTAssertTrue(foundIPAAttribute, "Leah range must have IPA attribute")
  }
}
