import XCTest
@testable import LeahUI

@MainActor
final class AppearancePaneTests: XCTestCase {
    override func setUp() {
        super.setUp()
        UserDefaults.standard.removeObject(forKey: AppearancePane.themeKey)
        UserDefaults.standard.removeObject(forKey: AppearancePane.minimalModeKey)
    }

    func testAppearanceToggleWritesUserDefaults() {
        XCTAssertEqual(AppearancePane.currentTheme(), .matchSystem,
                       "Default theme must be Match System")
        AppearancePane.setTheme(.light)
        XCTAssertEqual(UserDefaults.standard.string(forKey: AppearancePane.themeKey), "light")
        AppearancePane.setTheme(.dark)
        XCTAssertEqual(UserDefaults.standard.string(forKey: AppearancePane.themeKey), "dark")
        AppearancePane.setTheme(.matchSystem)
        XCTAssertEqual(UserDefaults.standard.string(forKey: AppearancePane.themeKey), "matchSystem")
    }

    func testMinimalModeWiringFlowsBoolean() {
        XCTAssertFalse(AppearancePane.minimalModeEnabled(),
                       "Minimal mode default must be OFF")
        AppearancePane.setMinimalModeEnabled(true)
        XCTAssertTrue(AppearancePane.minimalModeEnabled())
        XCTAssertEqual(UserDefaults.standard.bool(forKey: AppearancePane.minimalModeKey), true)
        AppearancePane.setMinimalModeEnabled(false)
        XCTAssertFalse(AppearancePane.minimalModeEnabled())
    }

    func testAccentBudgetInfoRowText() {
        XCTAssertEqual(AppearancePane.accentBudgetText,
                       "≤ 3 gold accents per render")
    }
}
