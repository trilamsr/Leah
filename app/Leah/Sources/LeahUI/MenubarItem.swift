import AppKit

// AppKit contract: NSStatusItem must mutate on the main thread.
@MainActor
public final class MenubarItem {
  public enum State: Equatable { case idle, listening, error }
  public private(set) var currentState: State = .idle
  private let surface: StatusBarSurface
  private let onClick: () -> Void
  private let proxy: ClickProxy

  public convenience init(onClick: @escaping () -> Void) {
    self.init(surface: NSStatusBarSurface(), onClick: onClick)
  }

  // Surface injection lets headless XCTest swap in a fake that does not
  // require a window server (NSStatusBar.statusItem returns nil under CI).
  public init(surface: StatusBarSurface, onClick: @escaping () -> Void) {
    self.surface = surface
    self.onClick = onClick
    self.proxy = ClickProxy(handler: onClick)
    surface.install(image: MenubarHexagon.image(filled: false, hasDot: false),
                    target: proxy,
                    action: #selector(ClickProxy.fire))
  }

  public func setState(_ s: State) {
    currentState = s
    switch s {
    case .idle:
      surface.setImage(MenubarHexagon.image(filled: false, hasDot: false))
    case .listening:
      surface.setImage(MenubarHexagon.image(filled: true, hasDot: false))
    case .error:
      surface.setImage(MenubarHexagon.image(filled: true, hasDot: true))
    }
  }
}

@MainActor
public protocol StatusBarSurface: AnyObject {
  func install(image: NSImage, target: AnyObject, action: Selector)
  func setImage(_ image: NSImage)
}

@MainActor
final class NSStatusBarSurface: StatusBarSurface {
  private var item: NSStatusItem?

  func install(image: NSImage, target: AnyObject, action: Selector) {
    let it = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
    it.button?.image = image
    it.button?.target = target
    it.button?.action = action
    // Retain target via associated object so it outlives the call site.
    objc_setAssociatedObject(it.button as Any, &Self.proxyKey, target, .OBJC_ASSOCIATION_RETAIN)
    self.item = it
  }

  func setImage(_ image: NSImage) {
    item?.button?.image = image
  }

  private static var proxyKey = 0
}

final class ClickProxy: NSObject {
  let handler: () -> Void
  init(handler: @escaping () -> Void) { self.handler = handler }
  @objc func fire() { handler() }
}
