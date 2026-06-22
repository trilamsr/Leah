import XCTest
@testable import LeahApp

final class LeahAppTests: XCTestCase {
  func testAppBundleIdentifier() {
    XCTAssertEqual(LeahApp.bundleIdentifier, "com.maydow.leah")
  }

  func testMinimumMacOSVersion() {
    XCTAssertEqual(LeahApp.minimumMacOS, "14.0")
  }
}
