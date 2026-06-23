import SwiftUI
import AppKit
import Carbon.HIToolbox
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
  private let dashboard: DashboardWindow
  private let dashboardHotkey: CommandDHotkey

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

    let registry = WidgetTileRegistry()
    registry.registerWave2Defaults()
    let resolver = DashboardTileResolver { slot in
      let env = WidgetEnvelope(
        widget: slot.rawValue,
        id: "dashboard-\(slot.rawValue)",
        size: "M"
      )
      return registry.view(for: env) ?? AnyView(Color.clear)
    }
    let dash = DashboardWindow(resolver: resolver)
    dashboard = dash
    dashboardHotkey = CommandDHotkey { Task { @MainActor in dash.toggle() } }
    dashboardHotkey.register()
  }

  var body: some Scene {
    Settings { EmptyView() }
  }
}

@MainActor
final class CommandDHotkey {
  private var ref: EventHotKeyRef?
  private let handler: () -> Void

  init(_ handler: @escaping () -> Void) { self.handler = handler }

  func register() {
    var eventType = EventTypeSpec(
      eventClass: OSType(kEventClassKeyboard),
      eventKind: OSType(kEventHotKeyPressed)
    )
    InstallEventHandler(
      GetApplicationEventTarget(),
      { _, evt, userData in
        var hid = EventHotKeyID()
        GetEventParameter(
          evt, EventParamName(kEventParamDirectObject), EventParamType(typeEventHotKeyID),
          nil, MemoryLayout<EventHotKeyID>.size, nil, &hid
        )
        if hid.signature == OSType(0x4C454148) && hid.id == 2 {
          let mgr = Unmanaged<CommandDHotkey>.fromOpaque(userData!).takeUnretainedValue()
          mgr.handler()
        }
        return noErr
      },
      1, &eventType,
      Unmanaged.passUnretained(self).toOpaque(),
      nil
    )
    var r: EventHotKeyRef?
    let id = EventHotKeyID(signature: OSType(0x4C454148), id: 2)
    RegisterEventHotKey(
      UInt32(kVK_ANSI_D),
      UInt32(cmdKey),
      id,
      GetApplicationEventTarget(),
      0,
      &r
    )
    ref = r
  }

  deinit { if let r = ref { UnregisterEventHotKey(r) } }
}

