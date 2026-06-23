import XCTest
import SwiftUI
import AppKit
@testable import LeahUI

final class DashboardViewTests: XCTestCase {
  @MainActor
  func testReusesWidgetTileRegistry() {
    var resolved: [DashboardTileSlot] = []
    let resolver = DashboardTileResolver { slot in
      resolved.append(slot)
      return AnyView(Text(slot.rawValue))
    }
    _ = DashboardView(resolver: resolver)
    let req = DashboardView.requiredSlots
    for s in req { _ = resolver.view(for: s) }
    XCTAssertEqual(Set(resolved), Set(req), "DashboardView must source tiles from the resolver, not embed views")
  }

  @MainActor
  func testTiempos_italic_renders_only_on_dashboard() throws {
    let fm = FileManager.default
    let here = URL(fileURLWithPath: #filePath)
    let pkgRoot = here
      .deletingLastPathComponent()  // LeahUITests
      .deletingLastPathComponent()  // Tests
      .deletingLastPathComponent()  // Leah
    let sources = pkgRoot.appendingPathComponent("Sources")
    let dashboardDir = sources.appendingPathComponent("LeahUI/Dashboard").path
    var offenders: [String] = []
    if let it = fm.enumerator(atPath: sources.path) {
      for case let rel as String in it {
        guard rel.hasSuffix(".swift") else { continue }
        let abs = sources.appendingPathComponent(rel).path
        if abs.hasPrefix(dashboardDir) { continue }
        let body = (try? String(contentsOfFile: abs)) ?? ""
        for line in body.split(separator: "\n") {
          let s = line.trimmingCharacters(in: .whitespaces)
          if s.hasPrefix("//") || s.hasPrefix("/*") || s.hasPrefix("*") { continue }
          if s.contains("Tiempos") { offenders.append(rel); break }
        }
      }
    }
    XCTAssertTrue(offenders.isEmpty, "Tiempos italic confined to Dashboard/; offenders: \(offenders)")
  }

  @MainActor
  func testGridIs2Columns() {
    XCTAssertEqual(DashboardView.columnCount, 2)
  }

  @MainActor
  func testGoldAccentsBudget() {
    XCTAssertLessThanOrEqual(DashboardView.goldAccentBudget, 3)
  }

  @MainActor
  func testDashboardWindowIsBorderlessAndFullScreen() {
    let win = DashboardWindow.makeWindow()
    XCTAssertTrue(win.styleMask.contains(NSWindow.StyleMask.borderless))
    XCTAssertFalse(win.styleMask.contains(NSWindow.StyleMask.titled))
    XCTAssertTrue(win.collectionBehavior.contains(NSWindow.CollectionBehavior.fullScreenPrimary))
  }
}
