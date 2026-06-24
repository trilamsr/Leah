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
  private let dashboardHotkey: DashboardHotkey

  @MainActor
  init() {
    let mode = PaletteObserver.modeFor(NSApp?.effectiveAppearance ?? NSAppearance(named: .darkAqua)!)
    let observer = PaletteObserver(initial: mode)
    observer.attach()
    paletteObserver = observer
    // Wizard runs BEFORE hotkey registration — trust signal at first launch.
    // Defer to next runloop tick — NSApp is mid-launch in App.init; window
    // would otherwise spawn but never gain focus (round 2 hunt 2026-06-23).
    let wiz = wizard
    DispatchQueue.main.async { wiz.presentIfNeeded() }

    let client = IPCClient()
    let fp = FocusPanelController(client: client)
    focusPanel = fp
    let hk = HotkeyManager { Task { @MainActor in fp.summon() } }
    // App launch must not fail if ⌥Space is taken — Settings surfaces the
    // conflict badge so the user can rebind.
    do {
      _ = try hk.register()
    } catch let HotkeyRegistrationError.conflict(other) {
      NSLog("HotkeyManager: ⌥Space conflict (other=%@)", other ?? "unknown")
    } catch {
      NSLog("HotkeyManager: register failed: %@", String(describing: error))
    }
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

    let resolver = DashboardTileResolver { slot in
      AnyView(DashboardSlotPlaceholder(slot: slot))
    }
    let dash = DashboardWindow(resolver: resolver)
    dashboard = dash
    dashboardHotkey = DashboardHotkey { Task { @MainActor in dash.toggle() } }
    dashboardHotkey.register()
  }

  var body: some Scene {
    Settings { EmptyView() }
  }
}

@MainActor
final class DashboardHotkey {
  private var ref: EventHotKeyRef?
  private let handler: () -> Void

  init(_ handler: @escaping () -> Void) { self.handler = handler }

  func register() {
    var eventType = EventTypeSpec(
      eventClass: OSType(kEventClassKeyboard),
      eventKind: OSType(kEventHotKeyPressed)
    )
    let installStatus = InstallEventHandler(
      GetApplicationEventTarget(),
      { _, evt, userData in
        guard let ud = userData else { return OSStatus(eventParameterNotFoundErr) }
        var hid = EventHotKeyID()
        GetEventParameter(
          evt, EventParamName(kEventParamDirectObject), EventParamType(typeEventHotKeyID),
          nil, MemoryLayout<EventHotKeyID>.size, nil, &hid
        )
        if hid.signature == OSType(0x4C454148) && hid.id == 2 {
          let mgr = Unmanaged<DashboardHotkey>.fromOpaque(ud).takeUnretainedValue()
          mgr.handler()
        }
        return noErr
      },
      1, &eventType,
      Unmanaged.passUnretained(self).toOpaque(),
      nil
    )
    if installStatus != noErr {
      NSLog("DashboardHotkey: InstallEventHandler failed status=%d", installStatus)
    }
    var r: EventHotKeyRef?
    let id = EventHotKeyID(signature: OSType(0x4C454148), id: 2)
    let registerStatus = RegisterEventHotKey(
      UInt32(kVK_ANSI_D),
      UInt32(cmdKey | shiftKey),
      id,
      GetApplicationEventTarget(),
      0,
      &r
    )
    if registerStatus != noErr {
      NSLog("DashboardHotkey: RegisterEventHotKey failed status=%d (⌘⇧D already taken)", registerStatus)
    }
    ref = r
  }

  deinit { if let r = ref { UnregisterEventHotKey(r) } }
}

struct DashboardSlotPlaceholder: View {
  let slot: DashboardTileSlot

  var body: some View {
    VStack(alignment: .leading, spacing: 8) {
      Text(slot.rawValue.capitalized)
        .font(.callout.weight(.medium))
        .foregroundColor(LeahPalette.ivory)
      Text("coming in Phase 4")
        .font(.caption)
        .foregroundColor(LeahPalette.textMuted)
      Spacer(minLength: 0)
    }
    .padding(16)
    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
  }
}

