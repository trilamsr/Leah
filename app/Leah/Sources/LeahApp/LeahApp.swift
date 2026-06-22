import SwiftUI
import LeahUI

@main
struct LeahApp: App {
  static let bundleIdentifier = "com.maydow.leah"
  static let minimumMacOS = "14.0"

  // Retained for app lifetime; click summons focus panel (Task 10 wires receiver).
  private let menubarItem = MenubarItem(onClick: {
    NotificationCenter.default.post(name: .leahSummonFocusPanel, object: nil)
  })

  var body: some Scene {
    WindowGroup {
      // Replaced by NSPanel-hosted FocusPanel in Task 9; placeholder window
      // keeps the SwiftUI App protocol satisfied during bootstrap.
      Color.clear.frame(width: 1, height: 1)
    }
  }
}
