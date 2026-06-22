import XCTest
@testable import LeahUI

final class Step5IntegrationsTests: XCTestCase {
    func testWizardStep5HasMailAndFiles() {
        let kinds = IntegrationStep.Integration.allCases
        XCTAssertEqual(kinds, [.calendar, .mail, .files])
    }

    func testStep5ReusesStatusGlyph() {
        XCTAssertEqual(StatusGlyph.symbol(for: .none), "○")
        XCTAssertEqual(StatusGlyph.symbol(for: .connected), "●")
    }
}
