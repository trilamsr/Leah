import XCTest
@testable import LeahUI

final class RecommendationsPaneTests: XCTestCase {
    @MainActor
    func testSpecRulesAreLockedAndCannotBeRemoved() {
        let specRow = AntiRuleRow(kind: "wake-word-on", reason: "spec §3.6", source: .spec, addedAt: Date())
        let opRow = AntiRuleRow(kind: "pin-widget", reason: "noisy", source: .operator_, addedAt: Date())
        let model = RecommendationsPaneModel(rules: [specRow, opRow])

        XCTAssertEqual(model.lockedRules.count, 1, "spec row must be in locked section")
        XCTAssertEqual(model.lockedRules.first?.kind, "wake-word-on")
        XCTAssertEqual(model.editableRules.count, 1, "operator row must be editable")

        XCTAssertFalse(model.removeEditable(specRow), "spec source row must be un-removable")
        XCTAssertEqual(model.rules.count, 2, "spec row must still be present after removal attempt")

        XCTAssertTrue(model.removeEditable(opRow))
        XCTAssertEqual(model.rules.count, 1, "operator row should drop after remove")
    }

    @MainActor
    func testLedgerCapsAt30() {
        let rows = (0..<50).map {
            SurfacedLedgerRow(id: Int64($0), kind: "pin-widget", body: "b", outcome: "ignored", at: Date())
        }
        let model = RecommendationsPaneModel(ledger: rows)
        XCTAssertEqual(model.recentLedger.count, 30, "ledger must cap at last 30 entries")
        XCTAssertEqual(model.recentLedger.first?.id, 0)
        XCTAssertEqual(model.recentLedger.last?.id, 29)
    }

    @MainActor
    func testKindToggleRoundTrips() {
        let model = RecommendationsPaneModel(kindsEnabled: ["pin-widget": true, "voice-on": false])
        XCTAssertEqual(model.kindsEnabled["pin-widget"], true)
        model.kindsEnabled["pin-widget"] = false
        XCTAssertEqual(model.kindsEnabled["pin-widget"], false)
    }

    func testAntiRuleSourceLockBit() {
        XCTAssertTrue(AntiRuleSource.spec.isLocked)
        XCTAssertFalse(AntiRuleSource.operator_.isLocked)
        XCTAssertFalse(AntiRuleSource.auto.isLocked)
    }

    func testPaneEnumIncludesRecommendations() {
        XCTAssertTrue(SettingsPane.allCases.contains(.recommendations),
                      "SettingsPane enum must register .recommendations case")
        XCTAssertEqual(SettingsPane.recommendations.title, "Recommendations")
    }
}
