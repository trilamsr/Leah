import SwiftUI

@main
struct LeahApp: App {
  static let bundleIdentifier = "com.maydow.leah"
  static let minimumMacOS = "14.0"

  var body: some Scene {
    WindowGroup {
      // Replaced by NSPanel-hosted FocusPanel in Task 9; placeholder window
      // keeps the SwiftUI App protocol satisfied during bootstrap.
      Color.clear.frame(width: 1, height: 1)
    }
  }
}
