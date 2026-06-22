import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class WeatherTileViewTests: XCTestCase {
  private func loadFixture() throws -> WidgetEnvelope {
    let url = Bundle.module.url(forResource: "weather-sf-forecast", withExtension: "json", subdirectory: "Fixtures")
    let data = try Data(contentsOf: XCTUnwrap(url))
    return try JSONDecoder().decode(WidgetEnvelope.self, from: data)
  }

  func testFixtureDecodesIntoWeatherPayload() throws {
    let env = try loadFixture()
    let payload = try WeatherPayload(props: env.props)
    XCTAssertEqual(payload.location, "San Francisco")
    XCTAssertEqual(payload.units, .imperial)
    XCTAssertEqual(payload.current.temp, 64)
    XCTAssertEqual(payload.forecast.count, 7)
  }

  func testConditionsMapToSFSymbols() {
    XCTAssertEqual(WeatherTileView.iconName(for: "clear"),         "sun.max.fill")
    XCTAssertEqual(WeatherTileView.iconName(for: "partly_cloudy"), "cloud.sun.fill")
    XCTAssertEqual(WeatherTileView.iconName(for: "cloudy"),        "cloud.fill")
    XCTAssertEqual(WeatherTileView.iconName(for: "rain"),          "cloud.rain.fill")
    XCTAssertEqual(WeatherTileView.iconName(for: "storm"),         "cloud.bolt.fill")
    XCTAssertEqual(WeatherTileView.iconName(for: "snow"),          "snow")
    XCTAssertEqual(WeatherTileView.iconName(for: "wind"),          "wind")
    XCTAssertEqual(WeatherTileView.iconName(for: "fog"),           "cloud.fog.fill")
    XCTAssertEqual(WeatherTileView.iconName(for: "unknown"),       "cloud.fill")
  }

  func testRenderedTextHasNoEmoji() throws {
    let env = try loadFixture()
    let payload = try WeatherPayload(props: env.props)
    let strings = WeatherTileView.renderedStrings(payload: payload)
    XCTAssertFalse(strings.isEmpty)
    for s in strings {
      for scalar in s.unicodeScalars {
        XCTAssertFalse(
          scalar.properties.isEmojiPresentation || scalar.properties.isEmoji && scalar.value > 0x1F000,
          "rendered weather text contains emoji scalar U+\(String(scalar.value, radix: 16)) in \"\(s)\""
        )
      }
    }
  }

  func testIconNamesNeverEmpty() throws {
    let env = try loadFixture()
    let payload = try WeatherPayload(props: env.props)
    let names = payload.forecast.map { WeatherTileView.iconName(for: $0.condition) }
    XCTAssertEqual(names.count, 7)
    XCTAssertTrue(names.allSatisfy { !$0.isEmpty && !$0.contains(" ") })
  }

  func testBodyRenders() throws {
    let env = try loadFixture()
    let payload = try WeatherPayload(props: env.props)
    let view = WeatherTileView(payload: payload)
    _ = view.body
  }
}
