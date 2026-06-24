import AppKit
import Carbon.HIToolbox

public enum HotkeyRegistrationError: Error, Equatable {
  case conflict(otherApp: String?)
  case denied
  case unknown(OSStatus)
}

public final class HotkeyToken {
  fileprivate let ref: EventHotKeyRef
  fileprivate init(ref: EventHotKeyRef) { self.ref = ref }
  deinit { UnregisterEventHotKey(ref) }
}

public final class HotkeyManager {
  // kVK_Space = 0x31
  public static let optionSpaceKeyCode: UInt32 = UInt32(kVK_Space)
  // Carbon optionKey mask (= 0x0800)
  public static let optionFlag: UInt32 = UInt32(optionKey)

  private var token: HotkeyToken?
  private var nextID: UInt32 = 1
  private let handler: () -> Void
  private var eventHandlerInstalled = false

  public init(_ handler: @escaping () -> Void) {
    self.handler = handler
  }

  /// Throws so callers (Settings, Wizard) can render a recovery affordance
  /// instead of silently swallowing the OSStatus.
  @discardableResult
  public func register(
    keyCode: UInt32 = HotkeyManager.optionSpaceKeyCode,
    modifiers: UInt32 = HotkeyManager.optionFlag
  ) throws -> HotkeyToken {
    installEventHandlerIfNeeded()
    let t = try Self.registerRaw(keyCode: keyCode, modifiers: modifiers, idValue: nextID)
    nextID &+= 1
    token = t
    return t
  }

  /// Dry-run: register + immediate unregister via discard. The token's deinit
  /// fires synchronously, so no residual Carbon state leaks.
  public func isConflict(keyCode: UInt32, modifiers: UInt32) -> Bool {
    do {
      _ = try Self.registerRaw(keyCode: keyCode, modifiers: modifiers, idValue: 0xFFFF)
      return false
    } catch HotkeyRegistrationError.conflict {
      return true
    } catch {
      // Non-conflict failures still mean "cannot bind" — UI must warn.
      return true
    }
  }

  private func installEventHandlerIfNeeded() {
    guard !eventHandlerInstalled else { return }
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
    } else {
      eventHandlerInstalled = true
    }
  }

  /// Static so the probe path doesn't require a long-lived event handler.
  private static func registerRaw(
    keyCode: UInt32,
    modifiers: UInt32,
    idValue: UInt32
  ) throws -> HotkeyToken {
    var ref: EventHotKeyRef?
    let id = EventHotKeyID(signature: OSType(0x4C454148), id: idValue) // 'LEAH'
    let status = RegisterEventHotKey(
      keyCode, modifiers, id,
      GetApplicationEventTarget(),
      0, &ref
    )
    if status == noErr, let r = ref {
      return HotkeyToken(ref: r)
    }
    // -9878 exists / -9879 invalid both mean the chord is unavailable; Carbon
    // does not expose the owning app, so otherApp is nil.
    if status == OSStatus(eventHotKeyExistsErr) || status == OSStatus(eventHotKeyInvalidErr) {
      throw HotkeyRegistrationError.conflict(otherApp: nil)
    }
    throw HotkeyRegistrationError.unknown(status)
  }
}
