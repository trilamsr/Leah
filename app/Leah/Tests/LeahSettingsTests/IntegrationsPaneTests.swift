import XCTest
@testable import LeahUI

final class IntegrationsPaneTests: XCTestCase {
    func testIntegrationsHasCalendarMailFiles() {
        let kinds = IntegrationRow.Kind.allCases
        XCTAssertEqual(kinds, [.calendar, .mail, .files])
    }

    func testRowStatusGlyphsCycle() {
        XCTAssertEqual(StatusGlyph.symbol(for: .connected), "●")
        XCTAssertEqual(StatusGlyph.symbol(for: .partial),   "△")
        XCTAssertEqual(StatusGlyph.symbol(for: .none),      "○")
    }

    func testSettingsPaneIncludesIntegrations() {
        XCTAssertTrue(SettingsPane.allCases.contains(.integrations))
    }
}
