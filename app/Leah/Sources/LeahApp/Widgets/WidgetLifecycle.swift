import AppKit

public enum WidgetLifecycle {
  public enum State: String { case spawning, live, refreshing, stale, error, dismissed }
  public enum Event { case mount, update, stale, error, dismiss }
  public enum Size: String { case small, medium, large }

  public static let staggerInterval: TimeInterval = 0.080

  public static func reservedHeight(for size: Size) -> CGFloat {
    switch size {
    case .small: return 84
    case .medium: return 168
    case .large: return 252
    }
  }

  public static func revealDelay(index: Int, reduceMotion: Bool) -> TimeInterval {
    guard !reduceMotion, index > 0 else { return 0 }
    return staggerInterval * TimeInterval(index)
  }

  public static func shouldAnimate(reduceMotion: Bool) -> Bool { !reduceMotion }

  public static func systemReduceMotion() -> Bool {
    NSWorkspace.shared.accessibilityDisplayShouldReduceMotion
  }

  public struct Machine {
    public private(set) var state: State = .spawning
    public let size: Size
    public var reservedHeight: CGFloat { WidgetLifecycle.reservedHeight(for: size) }

    public init(size: Size) { self.size = size }

    @discardableResult
    public mutating func apply(_ event: Event) -> Bool {
      guard let next = WidgetLifecycle.next(from: state, on: event) else { return false }
      state = next
      return true
    }
  }

  static func next(from state: State, on event: Event) -> State? {
    if state == .dismissed { return nil }
    switch event {
    case .mount:
      return state == .spawning ? .live : nil
    case .update:
      return state == .live ? .refreshing : nil
    case .stale:
      return state == .spawning ? nil : .stale
    case .error:
      return .error
    case .dismiss:
      return .dismissed
    }
  }
}
