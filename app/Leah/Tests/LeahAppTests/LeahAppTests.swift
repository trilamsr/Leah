import XCTest
@testable import LeahApp

final class LeahAppTests: XCTestCase {
  func testAppBundleIdentifier() {
    // Verify CFBundleIdentifier from Info.plist matches expected value.
    // Use FileManager to locate Info.plist relative to the test target's source directory.
    let testSourceDir = URL(fileURLWithPath: #file).deletingLastPathComponent()
    let infoPlistPath = testSourceDir.deletingLastPathComponent().deletingLastPathComponent()
      .appendingPathComponent("Sources/LeahApp/Info.plist").path

    guard let infoPlist = NSDictionary(contentsOfFile: infoPlistPath) as? [String: Any],
          let bundleId = infoPlist["CFBundleIdentifier"] as? String else {
      XCTFail("Could not read CFBundleIdentifier from Info.plist at \(infoPlistPath)")
      return
    }
    XCTAssertEqual(bundleId, "com.maydow.leah")
  }

  func testMinimumMacOSVersion() {
    XCTAssertEqual(LeahApp.minimumMacOS, "14.0")
  }
}
