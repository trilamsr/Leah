import XCTest
import AppKit
@testable import LeahUI

@MainActor
final class FakeStatusBarSurface: StatusBarSurface {
  var installedImage: NSImage?
  var installedTarget: AnyObject?
  var installedAction: Selector?
  var lastImage: NSImage?

  func install(image: NSImage, target: AnyObject, action: Selector) {
    installedImage = image
    installedTarget = target
    installedAction = action
    lastImage = image
  }

  func setImage(_ image: NSImage) {
    lastImage = image
  }
}

final class MenubarItemTests: XCTestCase {
  @MainActor
  func testMenubarItemDefaultsToIdle() {
    let fake = FakeStatusBarSurface()
    let m = MenubarItem(surface: fake, onClick: {})
    XCTAssertEqual(m.state, .idle)
    XCTAssertNotNil(fake.installedImage, "install() must run on construction")
  }

  @MainActor
  func testSetStateMutatesSurfaceImage() {
    let fake = FakeStatusBarSurface()
    let m = MenubarItem(surface: fake, onClick: {})
    let idleImg = fake.lastImage
    m.setState(.listening)
    XCTAssertNotNil(fake.lastImage)
    XCTAssertFalse(fake.lastImage === idleImg, "listening must swap image")
    m.setState(.error)
    XCTAssertNotNil(fake.lastImage)
  }

  @MainActor
  func testBindFollowsControllerPublishedState() {
    let fake = FakeStatusBarSurface()
    let m = MenubarItem(surface: fake, onClick: {})
    let ctrl = MenubarStateController()
    m.bind(to: ctrl)
    ctrl.transition(to: .thinking)
    RunLoop.main.run(until: Date().addingTimeInterval(0.05))
    XCTAssertEqual(m.state, .thinking)
  }
}
