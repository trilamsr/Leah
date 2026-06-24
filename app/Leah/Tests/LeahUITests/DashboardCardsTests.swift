import XCTest
import SwiftUI
import AppKit
@testable import LeahUI

final class DashboardCardsTests: XCTestCase {
  // MARK: - CoachCard

  @MainActor
  func testCoachCardExposesThreeCounterSlots() {
    // Surfaced / Dismissed / Applied — three slots per §3.12 dashboard surface.
    XCTAssertEqual(CoachCard.statSlots, [.surfaced, .dismissed, .applied])
  }

  @MainActor
  func testCoachCardCounterValuesMatchStats() {
    let stats = CoachStats(surfaced: 12, dismissed: 4, applied: 3)
    XCTAssertEqual(CoachCard.value(stats, for: .surfaced), 12)
    XCTAssertEqual(CoachCard.value(stats, for: .dismissed), 4)
    XCTAssertEqual(CoachCard.value(stats, for: .applied), 3)
  }

  @MainActor
  func testCoachCardTapTargetsRecommendationsPane() {
    XCTAssertEqual(CoachCard.tapTargetPane, "recommendations")
  }

  @MainActor
  func testCoachCardRendersWithoutCrashing() {
    let host = NSHostingView(rootView: CoachCard(stats: CoachStats(surfaced: 1, dismissed: 2, applied: 3)))
    host.frame = NSRect(x: 0, y: 0, width: 360, height: 200)
    host.layoutSubtreeIfNeeded()
  }

  // MARK: - PrivacyCard

  @MainActor
  func testPrivacyCardRendersAllSevenBuckets() {
    // §8.9: card surfaces the full 7-bucket privacy meter set from internal/budget.
    XCTAssertEqual(PrivacyCard.bucketOrder.count, 7)
    let names = Set(PrivacyCard.bucketOrder.map { $0.id })
    XCTAssertEqual(names, [
      "cloud.llm.tokens",
      "cloud.embed.bytes",
      "cloud.stt.seconds",
      "cloud.tts.chars",
      "cloud.vision.bytes",
      "peer.a2a.tokens",
      "plugin.network.bytes",
    ])
  }

  @MainActor
  func testPrivacyCardTrendIsSevenDays() {
    // Week trend ⇒ exactly 7 samples per bucket (oldest→newest).
    let trends = PrivacyCard.placeholderTrends()
    XCTAssertEqual(trends.count, 7)
    for t in trends {
      XCTAssertEqual(t.weekSpend.count, 7, "bucket \(t.id) must have 7 daily samples")
    }
  }

  @MainActor
  func testPrivacyCardTapTargetsBudgetsPane() {
    XCTAssertEqual(PrivacyCard.tapTargetPane, "privacy.budgets")
  }

  @MainActor
  func testPrivacyCardRendersWithoutCrashing() {
    let host = NSHostingView(rootView: PrivacyCard(trends: PrivacyCard.placeholderTrends()))
    host.frame = NSRect(x: 0, y: 0, width: 360, height: 200)
    host.layoutSubtreeIfNeeded()
  }

  // MARK: - HealthCard

  @MainActor
  func testHealthCardStatusEnumHasGreenYellowRed() {
    XCTAssertEqual(Set(HealthStatus.allCases), [.green, .yellow, .red])
  }

  @MainActor
  func testHealthCardRendersOneRowPerProcess() {
    let procs = [
      ProcessHealth(name: "leahd", status: .green),
      ProcessHealth(name: "voiced", status: .yellow),
      ProcessHealth(name: "syncd", status: .red),
    ]
    XCTAssertEqual(HealthCard.rowCount(procs), 3)
  }

  @MainActor
  func testHealthCardTapTargetsDiagnosticsPane() {
    XCTAssertEqual(HealthCard.tapTargetPane, "about.diagnostics")
  }

  @MainActor
  func testHealthCardRendersWithoutCrashing() {
    let host = NSHostingView(rootView: HealthCard(processes: [
      ProcessHealth(name: "leahd", status: .green),
      ProcessHealth(name: "voiced", status: .yellow),
    ]))
    host.frame = NSRect(x: 0, y: 0, width: 360, height: 200)
    host.layoutSubtreeIfNeeded()
  }

  // MARK: - DashboardView wiring

  @MainActor
  func testDashboardSlotsIncludeCoachPrivacyHealth() {
    let slots = Set(DashboardView.requiredSlots)
    XCTAssertTrue(slots.contains(.coach), "DashboardView must slot CoachCard (§3.12)")
    XCTAssertTrue(slots.contains(.privacy), "DashboardView must slot PrivacyCard (§8.9)")
    XCTAssertTrue(slots.contains(.health), "DashboardView must slot HealthCard (§9.10)")
  }

  // MARK: - Tap → notification plumbing

  @MainActor
  func testNotificationCarriesPaneIdentifier() {
    let exp = expectation(description: "leahOpenSettings fires with pane userInfo")
    let token = NotificationCenter.default.addObserver(forName: .leahOpenSettings, object: nil, queue: .main) { note in
      if (note.userInfo?[Notification.leahSettingsPaneKey] as? String) == "recommendations" {
        exp.fulfill()
      }
    }
    defer { NotificationCenter.default.removeObserver(token) }
    NotificationCenter.default.post(
      name: .leahOpenSettings,
      object: nil,
      userInfo: [Notification.leahSettingsPaneKey: "recommendations"]
    )
    wait(for: [exp], timeout: 1.0)
  }
}
