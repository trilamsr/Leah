import SwiftUI
import LeahUI
import LeahIPC

@main
struct LeahApp: App {
  static let bundleIdentifier = "com.maydow.leah"
  static let minimumMacOS = "14.0"

  // Retained for app lifetime.
  private let menubarItem = MenubarItem(onClick: {
    NotificationCenter.default.post(name: .leahSummonFocusPanel, object: nil)
  })
  private let focusPanel: FocusPanelController
  private let hotkey: HotkeyManager
  private let summonObserver: NSObjectProtocol
  private let settingsObserver: NSObjectProtocol
  private let wizard = WizardController()

  @MainActor
  init() {
    // Wizard runs BEFORE hotkey registration — trust signal at first launch.
    wizard.presentIfNeeded()

    let client = IPCClient()
    let fp = FocusPanelController(client: client)
    focusPanel = fp
    let hk = HotkeyManager { Task { @MainActor in fp.summon() } }
    hk.register()
    hotkey = hk
    summonObserver = NotificationCenter.default.addObserver(
      forName: .leahSummonFocusPanel,
      object: nil,
      queue: .main
    ) { _ in Task { @MainActor in fp.summon() } }
    let sw = SettingsWindowController()
    settingsObserver = NotificationCenter.default.addObserver(
      forName: .leahOpenSettings,
      object: nil,
      queue: .main
    ) { _ in Task { @MainActor in sw.show() } }
  }

  var body: some Scene {
    WindowGroup {
      // NSPanel-hosted FocusPanelView is the primary UI surface.
      // This placeholder satisfies the SwiftUI App protocol.
      Color.clear.frame(width: 1, height: 1)
    }
  }
}
