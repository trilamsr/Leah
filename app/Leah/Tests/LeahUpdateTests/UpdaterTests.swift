import XCTest
@testable import LeahUpdate

final class UpdaterTests: XCTestCase {
    func testFeedURLDefault() {
        let u = Updater(feedURL: "https://maydow.github.io/leah/appcast.xml")
        XCTAssertEqual(u.feedURL, "https://maydow.github.io/leah/appcast.xml")
    }
}
