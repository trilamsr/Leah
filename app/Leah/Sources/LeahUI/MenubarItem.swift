import AppKit

public final class MenubarItem {
  public enum State: Equatable { case idle, listening, error }
  public private(set) var currentState: State = .idle
  private var item: NSStatusItem?
  private let onClick: () -> Void

  public init(onClick: @escaping () -> Void) {
    self.onClick = onClick
    let it = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
    it.button?.image = MenubarHexagon.image(filled: false, hasDot: false)
    let proxy = ClickProxy(handler: onClick)
    it.button?.target = proxy
    it.button?.action = #selector(ClickProxy.fire)
    // retain proxy via associated object so it lives as long as the button
    objc_setAssociatedObject(it.button as Any, &MenubarItem.proxyKey, proxy, .OBJC_ASSOCIATION_RETAIN)
    self.item = it
  }

  public func setState(_ s: State) {
    currentState = s
    switch s {
    case .idle:
      item?.button?.image = MenubarHexagon.image(filled: false, hasDot: false)
    case .listening:
      item?.button?.image = MenubarHexagon.image(filled: true, hasDot: false)
    case .error:
      item?.button?.image = MenubarHexagon.image(filled: true, hasDot: true)
    }
  }

  private static var proxyKey = 0
}

private final class ClickProxy: NSObject {
  let handler: () -> Void
  init(handler: @escaping () -> Void) { self.handler = handler }
  @objc func fire() { handler() }
}
