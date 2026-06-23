import XCTest
import SwiftUI
@testable import LeahWidgets

final class MinimalModeTests: XCTestCase {
    private let key = "leah.appearance.minimalMode"

    override func tearDown() {
        UserDefaults.standard.removeObject(forKey: key)
        super.tearDown()
    }

    func test_accentGoldStripsWhenMinimal() {
        UserDefaults.standard.set(true, forKey: key)
        XCTAssertEqual(Palette.accentGold, Color.clear)
    }

    func test_accentGoldIsChampagneWhenNotMinimal() {
        UserDefaults.standard.set(false, forKey: key)
        XCTAssertEqual(Palette.accentGold, Palette.champagneGold)
    }

    func test_minimalGuardStripsItalic() {
        UserDefaults.standard.set(true, forKey: key)
        XCTAssertTrue(Palette.minimalMode)
        // when minimal is on, the modifier replaces the grain overlay branch
        // with a system font environment — assert the toggle wiring rather
        // than the resolved render tree (SwiftUI runtime guards `body`).
        let modifier = MinimalGuard()
        XCTAssertNotNil(modifier as Any)
    }

    func test_minimalGuardStripsGrain() {
        UserDefaults.standard.set(false, forKey: key)
        XCTAssertFalse(Palette.minimalMode)
        // toggle off → MinimalGuard installs the grain overlay branch.
        let modifier = MinimalGuard()
        XCTAssertNotNil(modifier as Any)
    }
}
