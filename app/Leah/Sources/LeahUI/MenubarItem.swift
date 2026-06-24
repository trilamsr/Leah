import AppKit
import Combine

// AppKit contract: NSStatusItem must mutate on the main thread.
@MainActor
public final class MenubarItem {
  @Published public private(set) var state: MenubarState = .idle
  private let surface: StatusBarSurface
  private let onClick: () -> Void
  private let proxy: ClickProxy
  private var stateSub: AnyCancellable?

  public convenience init(onClick: @escaping () -> Void) {
    self.init(surface: NSStatusBarSurface(), onClick: onClick)
  }

  // Surface injection lets headless XCTest swap in a fake that does not
  // require a window server (NSStatusBar.statusItem returns nil under CI).
  public init(surface: StatusBarSurface, onClick: @escaping () -> Void) {
    self.surface = surface
    self.onClick = onClick
    self.proxy = ClickProxy(handler: onClick)
    surface.install(image: MenubarHexagon.image(for: .idle),
                    target: proxy,
                    action: #selector(ClickProxy.fire))
  }

  public func setState(_ s: MenubarState) {
    state = s
    surface.setImage(MenubarHexagon.image(for: s))
  }

  /// Bind to a controller's state publisher; the menubar follows it on the
  /// main run loop. Replaces any previous subscription.
  public func bind(to controller: MenubarStateController) {
    stateSub = controller.$state
      .receive(on: RunLoop.main)
      .sink { [weak self] s in self?.setState(s) }
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
