import XCTest
@testable import LeahApp

@MainActor
final class CodeDiffFixturesTests: XCTestCase {
  func testCodeGoFixtureDecodes() throws {
    let env = try Self.loadFixture(named: "code-go-snippet.json")
    XCTAssertEqual(env.widget, "code")
    let payload = try CodeTilePayload(envelope: env)
    XCTAssertEqual(payload.language, "go")
    XCTAssertFalse(payload.source.isEmpty)
  }

  func testDiffFixtureDecodes() throws {
    let env = try Self.loadFixture(named: "diff-typo-fix.json")
    XCTAssertEqual(env.widget, "diff")
    let payload = try DiffTilePayload(envelope: env)
    XCTAssertTrue(payload.lines.contains { $0.kind == .added })
    XCTAssertTrue(payload.lines.contains { $0.kind == .removed })
  }

  func testRegistryHasCodeAndDiffBuilders() {
    let r = WidgetTileRegistry()
    r.registerWave2Defaults()
    let codeEnv = WidgetEnvelope(widget: "code", id: "x", size: "medium",
      props: ["language": AnyCodable("go"), "source": AnyCodable("func main(){}")])
    XCTAssertNotNil(r.view(for: codeEnv))
    let diffEnv = WidgetEnvelope(widget: "diff", id: "y", size: "medium",
      props: ["unified": AnyCodable("@@ -1,1 +1,1 @@\n-a\n+b\n")])
    XCTAssertNotNil(r.view(for: diffEnv))
  }

  static func loadFixture(named name: String) throws -> WidgetEnvelope {
    // Fixtures live alongside the test sources; resolved via #filePath.
    let testDir = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    let url = testDir.appendingPathComponent("Fixtures/\(name)")
    let data = try Data(contentsOf: url)
    return try JSONDecoder().decode(WidgetEnvelope.self, from: data)
  }
}
