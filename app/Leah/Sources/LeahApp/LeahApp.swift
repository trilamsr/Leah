import SwiftUI
import AppKit
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
  private let hud = AmbientHUDWindow()
  private let paletteObserver: PaletteObserver

  @MainActor
  init() {
    let mode = PaletteObserver.modeFor(NSApp?.effectiveAppearance ?? NSAppearance(named: .darkAqua)!)
    let observer = PaletteObserver(initial: mode)
    observer.attach()
    paletteObserver = observer
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
    // sw is retained by the observer closure for the observer's lifetime.
    let sw = SettingsWindowController()
    settingsObserver = NotificationCenter.default.addObserver(
      forName: .leahOpenSettings,
      object: nil,
      queue: .main
    ) { _ in Task { @MainActor in sw.show() } }
    hud.mount()
  }

  var body: some Scene {
    Settings { EmptyView() }
  }
}
