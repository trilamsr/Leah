import XCTest
@testable import LeahUI

final class ConnectionsPaneTests: XCTestCase {
    func testIntegrationsHasCalendarMailFiles() {
        let kinds = IntegrationRow.Kind.allCases
        XCTAssertEqual(kinds, [.calendar, .mail, .files])
    }

    func testRowStatusGlyphsCycle() {
        XCTAssertEqual(StatusGlyph.symbol(for: .connected), "●")
        XCTAssertEqual(StatusGlyph.symbol(for: .partial),   "△")
        XCTAssertEqual(StatusGlyph.symbol(for: .none),      "○")
    }

    func testSettingsPaneIncludesConnections() {
        XCTAssertTrue(SettingsPane.allCases.contains(.connections))
        XCTAssertEqual(SettingsPane.connections.title, "Connections")
        XCTAssertEqual(SettingsPane.connections.keyboardShortcut, "7")
    }

    func testInboundMCPScopeRawValuesMatchGoConstants() {
        // Wire contract — the raw strings cross IPC into the Go inbound.Scope
        // closed set (phase4 §5.3). Drift here silently breaks tokens.
        XCTAssertEqual(InboundMCPScope.memoryRead.rawValue,   "memory:read")
        XCTAssertEqual(InboundMCPScope.calendarRead.rawValue, "calendar:read")
        XCTAssertEqual(InboundMCPScope.repoRead.rawValue,     "repo:read")
        XCTAssertEqual(InboundMCPScope.askRun.rawValue,       "ask:run")
        XCTAssertEqual(InboundMCPScope.widgetRender.rawValue, "widget:render")
        XCTAssertEqual(InboundMCPScope.allCases.count, 5)
    }

    func testPreviewStoreIssuesPlainOnceAndRevokes() {
        let store = PreviewInboundMCPTokenStore()
        let issued = store.issue(name: "ci-bot", scopes: [.memoryRead])
        XCTAssertNotNil(issued.plain)
        XCTAssertEqual(issued.name, "ci-bot")
        XCTAssertEqual(store.listTokens().count, 1)
        store.revoke(id: issued.id)
        XCTAssertEqual(store.listTokens().count, 0)
    }
}
