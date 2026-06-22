import Foundation

public extension Notification.Name {
  // Posted by MenubarItem click; consumed by FocusPanelController (Task 10).
  static let leahSummonFocusPanel = Notification.Name("com.maydow.leah.summonFocusPanel")
  // Posted by wizard ReadyStep; consumed by LeahApp to open Settings.
  static let leahOpenSettings = Notification.Name("com.maydow.leah.openSettings")
}
