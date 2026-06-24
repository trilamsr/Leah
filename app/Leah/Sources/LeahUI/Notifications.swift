import Foundation

public extension Notification.Name {
  // Posted by MenubarItem click; consumed by FocusPanelController (Task 10).
  static let leahSummonFocusPanel = Notification.Name("com.maydow.leah.summonFocusPanel")
  // Posted by wizard ReadyStep + dashboard cards; consumed by LeahApp to open Settings.
  // userInfo[Notification.leahSettingsPaneKey] may carry a pane identifier (string).
  static let leahOpenSettings = Notification.Name("com.maydow.leah.openSettings")
  // Posted when ambient HUD's "+N more pinned" disclosure is tapped — capping
  // pins at 2 in ambient (§9.0) preserves the <2% screen-area promise; the
  // overflow has to land somewhere, and Dashboard owns the full pin list.
  static let leahOpenDashboard = Notification.Name("com.maydow.leah.openDashboard")
}

public extension Notification {
  // userInfo key for routing leahOpenSettings to a specific pane.
  static let leahSettingsPaneKey = "pane"
}
