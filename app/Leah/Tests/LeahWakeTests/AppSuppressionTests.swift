import XCTest
@testable import LeahWake

final class AppSuppressionTests: XCTestCase {
    func testTerminalNotSuppressed() {
        let s = AppSuppression(stubFrontmost: "com.apple.Terminal")
        XCTAssertFalse(s.suppressedRightNow())
    }

    func testDefaultBundlesAllSuppress() {
        for bundle in AppSuppression.defaultMeetingBundles {
            let s = AppSuppression(stubFrontmost: bundle)
            XCTAssertTrue(s.suppressedRightNow(), "\(bundle) should suppress")
        }
    }

    func testEmptyFrontmostNotSuppressed() {
        let s = AppSuppression(stubFrontmost: "")
        XCTAssertFalse(s.suppressedRightNow())
    }
}
