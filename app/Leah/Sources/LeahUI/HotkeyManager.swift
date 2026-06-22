import AppKit
import Carbon.HIToolbox

public final class HotkeyManager {
  // kVK_Space = 0x31
  public static let optionSpaceKeyCode: UInt32 = UInt32(kVK_Space)
  // Carbon optionKey mask (= 0x0800)
  public static let optionFlag: UInt32 = UInt32(optionKey)

  private var hotKeyRef: EventHotKeyRef?
  private let handler: () -> Void

  public init(_ handler: @escaping () -> Void) {
    self.handler = handler
  }

  /// Registers ⌥Space as a global hotkey via Carbon RegisterEventHotKey.
  /// Requires macOS Accessibility permission for the running bundle.
  public func register() {
    var eventType = EventTypeSpec(
      eventClass: OSType(kEventClassKeyboard),
      eventKind: OSType(kEventHotKeyPressed)
    )
    InstallEventHandler(
      GetApplicationEventTarget(),
      { _, _, userData in
        let mgr = Unmanaged<HotkeyManager>.fromOpaque(userData!).takeUnretainedValue()
        mgr.handler()
        return noErr
      },
      1, &eventType,
      Unmanaged.passUnretained(self).toOpaque(),
      nil
    )
    var ref: EventHotKeyRef?
    let id = EventHotKeyID(signature: OSType(0x4C454148), id: 1) // 'LEAH'
    RegisterEventHotKey(
      HotkeyManager.optionSpaceKeyCode,
      HotkeyManager.optionFlag,
      id,
      GetApplicationEventTarget(),
      0,
      &ref
    )
    hotKeyRef = ref
  }

  deinit {
    if let r = hotKeyRef { UnregisterEventHotKey(r) }
  }
}
