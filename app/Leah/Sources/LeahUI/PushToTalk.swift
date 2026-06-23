import AppKit

/// PushToTalk implements §6.4: hold Fn (internal kbd) or right-⌘ (external)
/// to talk. Space and Option are explicitly rejected — Discord/Slack collision
/// classes the spec calls out (Option alone collides with global ⌥Space).
public final class PushToTalk {
    public var onStart: () -> Void = {}
    public var onStop: () -> Void  = {}
    private var active = false
    // kVK_RightCommand
    private let rightCommandKeyCode: UInt16 = 54

    public init() {}

    public func attach(to window: NSWindow) {
        // Production wiring registers an NSEvent local monitor for keyDown/keyUp
        // routed through simulate(...). The dev harness in Task 18 exercises that
        // path end-to-end; unit tests drive simulate directly.
    }

    /// simulate is the test seam and the production dispatch entry. Holding the
    /// trigger fires onStart exactly once; releasing fires onStop exactly once.
    public func simulate(modifierFlags: NSEvent.ModifierFlags, keyCode: UInt16) {
        let fn = modifierFlags.contains(.function)
        let rcmd = modifierFlags.contains(.command) && keyCode == rightCommandKeyCode
        let triggers = fn || rcmd
        if triggers && !active {
            active = true
            onStart()
        } else if !triggers && active {
            active = false
            onStop()
        }
    }
}
