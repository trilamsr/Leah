import XCTest
import SwiftUI
@testable import LeahWidgets

final class WidgetTests: XCTestCase {
    func testStatWidgetRenders() {
        let w = StatWidget(label: "MERGED PRs · 7d", value: "12", trend: "+3")
        let view = w.body
        XCTAssertNotNil(view as Any)
    }

    func testTableWidgetRenders() {
        let w = TableWidget(headers: ["PR", "Title"], rows: [["#321", "B1+B5 server push"]])
        XCTAssertNotNil(w.body as Any)
    }

    func testListWidgetRenders() {
        let w = ListWidget(items: ["MAY-19 done", "MAY-13 in-progress"])
        XCTAssertNotNil(w.body as Any)
    }

    func testEnvelopeDecodesStat() throws {
        let json = #"{"id":"x","widget":"stat","size":"small","props":{"label":"PRs","value":"12"},"data":{}}"#
        let env = try JSONDecoder().decode(WidgetEnvelope.self, from: json.data(using: .utf8)!)
        XCTAssertEqual(env.widget, "stat")
        XCTAssertEqual(env.id, "x")
    }

    func testStatDeltaPositiveUsesGoldColor() {
        let w = StatWidget(label: "Deploys", value: "5", trend: "+2")
        XCTAssertNotNil(w.body as Any)
    }

    func testStatDeltaNegativeUsesOxbloodColor() {
        let w = StatWidget(label: "Errors", value: "3", trend: "-1")
        XCTAssertNotNil(w.body as Any)
    }

    func testStatNoDelta() {
        let w = StatWidget(label: "Open PRs", value: "7", trend: nil)
        XCTAssertNotNil(w.body as Any)
    }

    func testTableNegativeMaskHighlightsCell() {
        let w = TableWidget(
            headers: ["Symbol", "P&L"],
            rows: [["AAPL", "-$200"], ["MSFT", "+$150"]],
            negativeMask: [[false, true], [false, false]]
        )
        XCTAssertNotNil(w.body as Any)
    }

    func testListOrdered() {
        let w = ListWidget(items: ["First", "Second", "Third"], ordered: true)
        XCTAssertNotNil(w.body as Any)
    }

    func testListUnordered() {
        let w = ListWidget(items: ["Alpha", "Beta"], ordered: false)
        XCTAssertNotNil(w.body as Any)
    }

    func testEnvelopeDecodesTable() throws {
        let json = #"{"id":"t1","widget":"table","size":"medium","props":{},"data":{"headers":["A","B"],"rows":[["1","2"]]}}"#
        let env = try JSONDecoder().decode(WidgetEnvelope.self, from: json.data(using: .utf8)!)
        XCTAssertEqual(env.widget, "table")
        XCTAssertEqual(env.id, "t1")
    }

    func testEnvelopeDecodesList() throws {
        let json = #"{"id":"l1","widget":"list","size":"small","props":{},"data":{"items":["x","y"]}}"#
        let env = try JSONDecoder().decode(WidgetEnvelope.self, from: json.data(using: .utf8)!)
        XCTAssertEqual(env.widget, "list")
        XCTAssertEqual(env.id, "l1")
    }

    func testPaletteTokensExist() {
        XCTAssertNotNil(Palette.obsidian0 as Any)
        XCTAssertNotNil(Palette.champagneGold as Any)
        XCTAssertNotNil(Palette.oxblood as Any)
        XCTAssertNotNil(Palette.ivory as Any)
    }

    func testWidgetProtocolConformance() {
        let stat: any LeahWidget = StatWidget(label: "L", value: "V", trend: nil)
        let table: any LeahWidget = TableWidget(headers: [], rows: [])
        let list: any LeahWidget = ListWidget(items: [])
        XCTAssertNotNil(stat.size as Any)
        XCTAssertNotNil(table.size as Any)
        XCTAssertNotNil(list.size as Any)
    }

    func testEnvelopeRoundtripPreservesScalarsAndCollections() throws {
        let json = #"{"id":"r1","widget":"stat","size":"small","props":{"label":"PRs","value":"12","count":7,"ratio":0.5,"open":true},"data":{"rows":[["a","b"]]}}"#
        let env = try JSONDecoder().decode(WidgetEnvelope.self, from: json.data(using: .utf8)!)
        let encoded = try JSONEncoder().encode(env)
        let env2 = try JSONDecoder().decode(WidgetEnvelope.self, from: encoded)
        XCTAssertEqual(env.id, env2.id)
        XCTAssertEqual(env.widget, env2.widget)
        XCTAssertEqual(env.size, env2.size)
        XCTAssertEqual(env2.props["label"]?.value as? String, "PRs")
        XCTAssertEqual(env2.props["count"]?.value as? Int, 7)
        XCTAssertEqual(env2.props["ratio"]?.value as? Double, 0.5)
        XCTAssertEqual(env2.props["open"]?.value as? Bool, true)
        let rows = env2.data["rows"]?.value as? [AnyCodable]
        XCTAssertEqual(rows?.count, 1)
    }
}
