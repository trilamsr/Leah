import XCTest
@testable import LeahUI

final class SettingsTests: XCTestCase {
    func testFourPanesEnumerated() {
        XCTAssertEqual(SettingsPane.allCases.count, 4)
        XCTAssertTrue(SettingsPane.allCases.contains(.general))
        XCTAssertTrue(SettingsPane.allCases.contains(.privacy))
        XCTAssertTrue(SettingsPane.allCases.contains(.permissions))
        XCTAssertTrue(SettingsPane.allCases.contains(.advanced))
    }

    func testPaneTitles() {
        XCTAssertEqual(SettingsPane.general.title, "General")
        XCTAssertEqual(SettingsPane.privacy.title, "Privacy")
        XCTAssertEqual(SettingsPane.permissions.title, "Permissions")
        XCTAssertEqual(SettingsPane.advanced.title, "Advanced")
    }

    func testOpusDefaultsFalse() {
        let defaults = UserDefaults.standard
        let singleShot = defaults.object(forKey: "leah.opus.nextReply") as? Bool ?? false
        let sessionWide = defaults.object(forKey: "leah.opus.sessionWide") as? Bool ?? false
        XCTAssertFalse(singleShot, "Opus single-shot default must be false")
        XCTAssertFalse(sessionWide, "Opus session-wide default must be false")
    }

    func testSearchFilters() {
        let query = "gen"
        let matches = SettingsPane.allCases.filter { $0.title.lowercased().contains(query.lowercased()) }
        XCTAssertEqual(matches.count, 1)
        XCTAssertEqual(matches.first, .general)
    }
}
