import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class ImageTileViewTests: XCTestCase {
  private func env(props: [String: AnyCodable], id: String = "img-1") -> WidgetEnvelope {
    WidgetEnvelope(widget: "image", id: id, size: "medium", refresh: nil, actions: nil, props: props)
  }

  func testRejectsURLPropKey() {
    let e = env(props: ["url": AnyCodable("https://evil.example/x.png")])
    XCTAssertThrowsError(try ImageTileView.resolvePath(from: e)) { err in
      XCTAssertEqual(err as? ImageTileView.PathError, .urlFieldForbidden)
    }
  }

  func testRejectsHTTPInCachedPath() {
    let e = env(props: ["cached_path": AnyCodable("https://cdn.example/x.png")])
    XCTAssertThrowsError(try ImageTileView.resolvePath(from: e)) { err in
      XCTAssertEqual(err as? ImageTileView.PathError, .urlFieldForbidden)
    }
  }

  func testRejectsHTTPSchemeAnyKey() {
    let e = env(props: ["src": AnyCodable("http://example.com/y.png")])
    XCTAssertThrowsError(try ImageTileView.resolvePath(from: e)) { err in
      XCTAssertEqual(err as? ImageTileView.PathError, .urlFieldForbidden)
    }
  }

  func testRequiresCachedPath() {
    let e = env(props: ["mime": AnyCodable("image/png")])
    XCTAssertThrowsError(try ImageTileView.resolvePath(from: e)) { err in
      XCTAssertEqual(err as? ImageTileView.PathError, .cachedPathRequired)
    }
  }

  func testAcceptsCachedPath() throws {
    let e = env(props: ["cached_path": AnyCodable("/tmp/leah-image-cache/sha256-abc.png")])
    let path = try ImageTileView.resolvePath(from: e)
    XCTAssertEqual(path, "/tmp/leah-image-cache/sha256-abc.png")
  }

  func testBrokenBadgeWhenFileMissing() {
    XCTAssertEqual(ImageTileView.loadState(path: "/nonexistent/leah/missing-\(UUID().uuidString).png"), .broken)
  }

  func testLoadStateOKForExistingFile() throws {
    let dir = FileManager.default.temporaryDirectory.appendingPathComponent("leah-img-tests-\(UUID().uuidString)")
    try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: dir) }
    // 1x1 PNG (smallest valid)
    let png: [UInt8] = [
      0x89,0x50,0x4E,0x47,0x0D,0x0A,0x1A,0x0A,
      0x00,0x00,0x00,0x0D,0x49,0x48,0x44,0x52,
      0x00,0x00,0x00,0x01,0x00,0x00,0x00,0x01,
      0x08,0x06,0x00,0x00,0x00,0x1F,0x15,0xC4,
      0x89,0x00,0x00,0x00,0x0D,0x49,0x44,0x41,
      0x54,0x78,0x9C,0x63,0x00,0x01,0x00,0x00,
      0x05,0x00,0x01,0x0D,0x0A,0x2D,0xB4,0x00,
      0x00,0x00,0x00,0x49,0x45,0x4E,0x44,0xAE,
      0x42,0x60,0x82
    ]
    let f = dir.appendingPathComponent("ok.png")
    try Data(png).write(to: f)
    XCTAssertEqual(ImageTileView.loadState(path: f.path), .ok)
  }

  func testFixtureRejectsURLField() throws {
    let url = try XCTUnwrap(Bundle.module.url(forResource: "image-cached", withExtension: "json", subdirectory: "Fixtures"))
    let data = try Data(contentsOf: url)
    let env = try JSONDecoder().decode(WidgetEnvelope.self, from: data)
    XCTAssertNil(env.props["url"], "image fixture must not carry a url field")
    XCTAssertNotNil(env.props["cached_path"])
  }

  func testRegistryRendersImage() {
    let r = WidgetTileRegistry()
    ImageTileView.register(in: r)
    let e = env(props: ["cached_path": AnyCodable("/tmp/leah-img/none.png")])
    XCTAssertNotNil(r.view(for: e))
  }
}
