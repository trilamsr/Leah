import Combine
import Foundation

/// Publishes daemon-state changes so the menubar glyph (or any other
/// surface) can subscribe instead of polling. Daemon IPC layer pushes
/// transitions in; observers (MenubarItem.bind) get them through Combine.
@MainActor
public final class MenubarStateController: ObservableObject {
  @Published public private(set) var state: MenubarState = .idle

  public init(initial: MenubarState = .idle) {
    self.state = initial
  }

  public func transition(to next: MenubarState) {
    state = next
  }
}
