import XCTest
import SwiftUI
import AppKit
@testable import LeahUI

@MainActor
final class PaneStateTests: XCTestCase {
    private func render<V: View>(_ view: V) {
        let host = NSHostingView(rootView: view)
        host.frame = NSRect(x: 0, y: 0, width: 480, height: 360)
        host.layoutSubtreeIfNeeded()
    }

    func testEmptyStateRenders() {
        render(EmptyState(symbol: "tray", title: "Empty", caption: "Nothing yet."))
    }

    func testErrorStateRenders() {
        render(ErrorState(message: "Boom.") {})
    }

    func testGeneralPaneRendersAllStates() {
        render(GeneralPane(initialState: .loaded))
        render(GeneralPane(initialState: .empty))
        render(GeneralPane(initialState: .error("daemon offline")))
    }

    func testAppearancePaneRendersAllStates() {
        render(AppearancePane(initialState: .loaded))
        render(AppearancePane(initialState: .empty))
        render(AppearancePane(initialState: .error("bad theme")))
    }

    func testMemoryPaneRendersAllStates() {
        let gate = StubGate()
        render(MemoryPane(gate: gate, initialState: .loaded))
        render(MemoryPane(gate: gate, initialState: .empty))
        render(MemoryPane(gate: gate, initialState: .error("db locked")))
    }

    func testVoicePaneRendersAllStates() {
        render(VoicePane(initialState: .loaded))
        render(VoicePane(initialState: .empty))
        render(VoicePane(initialState: .error("no audio device")))
    }

    func testPrivacyPaneRendersAllStates() {
        let gate = StubTelemetryGate()
        render(PrivacyPane(gate: gate, initialState: .loaded))
        render(PrivacyPane(gate: gate, initialState: .empty))
        render(PrivacyPane(gate: gate, initialState: .error("auth backend missing")))
    }

    func testConnectionsPaneRendersAllStates() {
        render(ConnectionsPane(initialState: .loaded))
        render(ConnectionsPane(initialState: .empty))
        render(ConnectionsPane(initialState: .error("EventKit unavailable")))
    }

    func testPermissionsPaneRendersAllStates() {
        render(PermissionsPane(initialState: .loaded))
        render(PermissionsPane(initialState: .empty))
        render(PermissionsPane(initialState: .error("TCC denied")))
    }

    func testAdvancedPaneRendersAllStates() {
        render(AdvancedPane(initialState: .loaded))
        render(AdvancedPane(initialState: .empty))
        render(AdvancedPane(initialState: .error("daemon unreachable")))
    }

    func testAboutPaneRendersAllStates() {
        render(AboutPane(initialState: .loaded))
        render(AboutPane(initialState: .empty))
        render(AboutPane(initialState: .error("bundle missing")))
    }

    func testRecommendationsPaneRendersAllStates() {
        render(RecommendationsPane(initialState: .loaded))
        render(RecommendationsPane(initialState: .empty))
        render(RecommendationsPane(initialState: .error("ledger unreadable")))
    }
}

private final class StubGate: TouchIDGating {
    var biometricsAvailable: Bool { false }
    func evaluate(reason: String) async -> Bool { false }
}

private final class StubTelemetryGate: TelemetryGating {
    var biometricsAvailable: Bool { false }
    func evaluate(reason: String) async -> Bool { false }
    func evaluateWithPasswordFallback(reason: String) async -> Bool { false }
}
