import XCTest
import SwiftUI
@testable import LeahApp

@MainActor
final class CitationTileViewTests: XCTestCase {
  private func env(props: [String: AnyCodable], id: String = "cite-1") -> WidgetEnvelope {
    WidgetEnvelope(widget: "citation", id: id, size: "small", refresh: nil, actions: nil, props: props)
  }

  func testTitleUsesNewYorkItalicSerif() {
    XCTAssertEqual(CitationTileView.titleFont, Font.system(.body, design: .serif).italic())
  }

  func testGoldAccentSurfaceLimitedToSourceDomain() {
    XCTAssertLessThanOrEqual(CitationTileView.goldSurfaceCount, 3)
  }

  func testSnippetTruncatesAtFourHundredChars() {
    let long = String(repeating: "a", count: 600)
    let out = CitationTileView.truncate(long, max: 400)
    XCTAssertEqual(out.count, 400)
  }

  func testSnippetShortPassesThrough() {
    XCTAssertEqual(CitationTileView.truncate("short", max: 400), "short")
  }

  func testStaleBadgeOnEnvelopeError() {
    let e = env(props: ["site_name": AnyCodable("arxiv.org"), "title": AnyCodable("T"), "error": AnyCodable("stale")])
    XCTAssertEqual(CitationTileView.badge(for: e), .stale)
  }

  func testFourOhFourBadgeOnStatusCode() {
    let e = env(props: ["site_name": AnyCodable("arxiv.org"), "title": AnyCodable("T"), "status_code": AnyCodable(404)])
    XCTAssertEqual(CitationTileView.badge(for: e), .notFound)
  }

  func testNoBadgeOnHealthyEnvelope() {
    let e = env(props: ["site_name": AnyCodable("arxiv.org"), "title": AnyCodable("T")])
    XCTAssertNil(CitationTileView.badge(for: e))
  }

  func testFixtureRoundTrips() throws {
    let url = try XCTUnwrap(Bundle.module.url(forResource: "citation-arxiv", withExtension: "json", subdirectory: "Fixtures"))
    let data = try Data(contentsOf: url)
    let env = try JSONDecoder().decode(WidgetEnvelope.self, from: data)
    XCTAssertEqual(env.widget, "citation")
    XCTAssertEqual(env.props["site_name"]?.value as? String, "arxiv.org")
  }

  func testRegistryRendersCitation() {
    let r = WidgetTileRegistry()
    CitationTileView.register(in: r)
    let e = env(props: ["site_name": AnyCodable("arxiv.org"), "title": AnyCodable("T"), "snippet": AnyCodable("s")])
    XCTAssertNotNil(r.view(for: e))
  }
}
