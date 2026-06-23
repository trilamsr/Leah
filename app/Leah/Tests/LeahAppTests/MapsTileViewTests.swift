import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class MapsTileViewTests: XCTestCase {
  private func loadFixture() throws -> MapsPayload {
    let url = try XCTUnwrap(Bundle.module.url(forResource: "maps-sf-geocode", withExtension: "json", subdirectory: "Fixtures"))
    let data = try Data(contentsOf: url)
    return try JSONDecoder().decode(MapsPayload.self, from: data)
  }

  func testDecodesLatLonLabelOnly() throws {
    let p = try loadFixture()
    XCTAssertEqual(p.lat, 37.8199, accuracy: 0.0001)
    XCTAssertEqual(p.lon, -122.4783, accuracy: 0.0001)
    XCTAssertEqual(p.label, "Golden Gate Bridge")
  }

  func testPayloadRejectsTileOrSnapshotFields() throws {
    // Daemon NEVER returns tiles or snapshots (§10.8). Decoder must succeed
    // on a payload that includes a stray field — but the model must not read
    // it. We assert no such property exists on MapsPayload at all.
    let mirror = Mirror(reflecting: MapsPayload(lat: 0, lon: 0, label: ""))
    let names = mirror.children.compactMap { $0.label?.lowercased() }
    for forbidden in ["tile", "tiles", "snapshot", "image", "imagedata", "tiledata"] {
      XCTAssertFalse(names.contains(where: { $0.contains(forbidden) }),
                     "MapsPayload exposes forbidden field \(forbidden)")
    }
  }

  func testJSONDecoderIgnoresExtraTileFieldsRatherThanReadingThem() throws {
    let polluted = """
    {"lat": 37.8, "lon": -122.4, "label": "X",
     "tile": "AAAA", "snapshot": "BBBB", "tile_data": "CCCC"}
    """.data(using: .utf8)!
    let p = try JSONDecoder().decode(MapsPayload.self, from: polluted)
    XCTAssertEqual(p.lat, 37.8, accuracy: 0.0001)
    XCTAssertEqual(p.lon, -122.4, accuracy: 0.0001)
    XCTAssertEqual(p.label, "X")
  }

  func testModelHasSingleAnnotation() throws {
    let p = try loadFixture()
    let m = MapsTileModel(payload: p)
    XCTAssertEqual(m.annotations.count, 1, "single destination pin only (§10.1 #5)")
    XCTAssertEqual(m.annotations[0].coordinate.latitude, p.lat, accuracy: 0.0001)
    XCTAssertEqual(m.annotations[0].coordinate.longitude, p.lon, accuracy: 0.0001)
  }

  func testGoldLandsOnDestinationPinOnly() throws {
    let p = try loadFixture()
    let m = MapsTileModel(payload: p)
    XCTAssertEqual(m.goldRegionCount, 1, "gold lands on destination pin only")
  }

  func testInstantiateView() throws {
    let p = try loadFixture()
    _ = MapsTileView(payload: p)
  }
}
