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
  func testIdleHexagonIsOutlinedTemplate() {
    let img = MenubarHexagon.image(filled: false, hasDot: false)
    XCTAssertTrue(img.isTemplate, "menubar images MUST be template per spec §4.2")
    XCTAssertEqual(img.size.width, 18, accuracy: 1)
    XCTAssertEqual(img.size.height, 18, accuracy: 1)
  }

  func testListeningHexagonIsFilledTemplate() {
    let img = MenubarHexagon.image(filled: true, hasDot: false)
    XCTAssertTrue(img.isTemplate)
  }

  func testErrorHexagonHasInnerDotTemplate() {
    let img = MenubarHexagon.image(filled: true, hasDot: true)
    XCTAssertTrue(img.isTemplate)
  }

  @MainActor
  func testMenubarItemDefaultsToIdle() {
    let fake = FakeStatusBarSurface()
    let m = MenubarItem(surface: fake, onClick: {})
    XCTAssertEqual(m.currentState, .idle)
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
}
