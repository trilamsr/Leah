import XCTest
@testable import LeahUI

final class AboutPaneTests: XCTestCase {
    func testAboutVersionMatchesBundle() {
        let expected = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.0.0"
        XCTAssertEqual(AboutInfo.version, expected)
    }

    func testCommitSHAExposed() {
        XCTAssertFalse(AboutInfo.commitSHA.isEmpty)
    }

    func testSettingsPaneIncludesAbout() {
        XCTAssertTrue(SettingsPane.allCases.contains(.about))
    }
}
