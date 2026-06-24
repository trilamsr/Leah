import XCTest
import SwiftUI
import AppKit
@testable import LeahUI

final class DashboardViewTests: XCTestCase {
  @MainActor
  func testReusesWidgetTileRegistry() {
    let resolver = DashboardTileResolver { slot in AnyView(Text(slot.rawValue)) }
    let host = NSHostingView(rootView: DashboardView(resolver: resolver))
    host.frame = NSRect(x: 0, y: 0, width: 1180, height: 760)
    host.layoutSubtreeIfNeeded()
    let want = Set(DashboardView.requiredSlots)
    let got = Set(resolver.callCount.keys.filter { resolver.callCount[$0, default: 0] > 0 })
    XCTAssertEqual(got, want, "DashboardView body must call resolver.view(for:) for every required slot")
  }

  @MainActor
  func testTiempos_italic_renders_only_on_dashboard() throws {
    let fm = FileManager.default
    let here = URL(fileURLWithPath: #filePath)
    let pkgRoot = here
      .deletingLastPathComponent()
      .deletingLastPathComponent()
      .deletingLastPathComponent()
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
  func testRequiredSlotsMatchSpec() {
    XCTAssertEqual(
      DashboardView.requiredSlots,
      [.memory, .agenda, .briefs, .news, .knowledge, .coach, .privacy, .health]
    )
  }

  @MainActor
  func testDashboardWindowIsResizableWith1180x760Default() {
    let win = DashboardWindow.makeWindow()
    XCTAssertTrue(win.styleMask.contains(NSWindow.StyleMask.titled))
    XCTAssertTrue(win.styleMask.contains(NSWindow.StyleMask.resizable))
    XCTAssertTrue(win.styleMask.contains(NSWindow.StyleMask.closable))
    XCTAssertEqual(DashboardWindow.defaultSize, NSSize(width: 1180, height: 760))
    XCTAssertEqual(win.frameAutosaveName, DashboardWindow.frameAutosaveName)
  }
}
