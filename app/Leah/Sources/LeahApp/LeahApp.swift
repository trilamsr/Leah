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

  init() {
    let client = IPCClient()
    let fp = FocusPanelController(client: client)
    focusPanel = fp
    let hk = HotkeyManager { Task { @MainActor in fp.summon() } }
    do {
      try hk.register()
    } catch {
      fatalError("Hotkey registration failed: \(error)")
    }
    hotkey = hk
    summonObserver = NotificationCenter.default.addObserver(
      forName: .leahSummonFocusPanel,
      object: nil,
      queue: .main
    ) { _ in Task { @MainActor in fp.summon() } }
  }

  var body: some Scene {
    WindowGroup {
      // NSPanel-hosted FocusPanelView is the primary UI surface.
      // This placeholder satisfies the SwiftUI App protocol.
      Color.clear.frame(width: 1, height: 1)
    }
  }
}
