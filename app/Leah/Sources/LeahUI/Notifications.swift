import Foundation

public extension Notification.Name {
  // Posted by MenubarItem click; consumed by FocusPanelController (Task 10).
  static let leahSummonFocusPanel = Notification.Name("com.maydow.leah.summonFocusPanel")
  // Posted by wizard ReadyStep + dashboard cards; consumed by LeahApp to open Settings.
  // userInfo[Notification.leahSettingsPaneKey] may carry a pane identifier (string).
  static let leahOpenSettings = Notification.Name("com.maydow.leah.openSettings")
}

public extension Notification {
  // userInfo key for routing leahOpenSettings to a specific pane.
  static let leahSettingsPaneKey = "pane"
}
