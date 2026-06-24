import XCTest
@testable import LeahApp

final class DemoPromptLibraryTests: XCTestCase {
  func testRegularLibraryHasSixPrompts() {
    XCTAssertEqual(DemoPromptLibrary.regular.count, 6)
  }

  func testAllPromptFieldsNonEmpty() {
    for p in DemoPromptLibrary.regular {
      XCTAssertFalse(p.icon.isEmpty, "icon empty for title=\(p.title)")
      XCTAssertFalse(p.title.isEmpty, "title empty for icon=\(p.icon)")
      XCTAssertFalse(p.prompt.isEmpty, "prompt empty for title=\(p.title)")
    }
  }

  @MainActor
  func testChipTapFiresCallback() {
    let sample = DemoPromptLibrary.regular[0]
    var received: DemoPrompt?
    let chip = DemoPromptChipView(prompt: sample) { received = sample }
    // Invoke the closure the chip would call on tap.
    chip.onTap()
    XCTAssertEqual(received, sample)
  }
}
