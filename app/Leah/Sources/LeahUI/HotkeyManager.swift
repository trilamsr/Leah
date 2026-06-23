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
    let installStatus = InstallEventHandler(
      GetApplicationEventTarget(),
      { _, _, userData in
        guard let ud = userData else { return OSStatus(eventParameterNotFoundErr) }
        let mgr = Unmanaged<HotkeyManager>.fromOpaque(ud).takeUnretainedValue()
        mgr.handler()
        return noErr
      },
      1, &eventType,
      Unmanaged.passUnretained(self).toOpaque(),
      nil
    )
    if installStatus != noErr {
      NSLog("HotkeyManager: InstallEventHandler failed status=%d (Accessibility permission likely missing)", installStatus)
    }
    var ref: EventHotKeyRef?
    let id = EventHotKeyID(signature: OSType(0x4C454148), id: 1) // 'LEAH'
    let registerStatus = RegisterEventHotKey(
      HotkeyManager.optionSpaceKeyCode,
      HotkeyManager.optionFlag,
      id,
      GetApplicationEventTarget(),
      0,
      &ref
    )
    if registerStatus != noErr {
      NSLog("HotkeyManager: RegisterEventHotKey failed status=%d (hotkey already taken by another app)", registerStatus)
    }
    hotKeyRef = ref
  }

  deinit {
    if let r = hotKeyRef { UnregisterEventHotKey(r) }
  }
}
