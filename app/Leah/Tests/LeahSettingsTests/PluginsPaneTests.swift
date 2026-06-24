import XCTest
@testable import LeahUI

final class PluginsPaneTests: XCTestCase {
    func testActionsStubReturnsEmpty() {
        XCTAssertTrue(PluginsPaneActions.stub.list().isEmpty)
    }

    func testEnableDispatchesIdToAction() {
        var enabledIds: [String] = []
        let actions = PluginsPaneActions(
            list: { [PluginRow(id: "com.x.y", name: "Y", version: "1", enabled: false, attestState: "good")] },
            enable: { enabledIds.append($0) },
            disable: { _ in },
            uninstall: { _ in },
            install: { _ in },
            logs: { _ in }
        )
        actions.enable("com.x.y")
        XCTAssertEqual(enabledIds, ["com.x.y"])
    }

    func testPluginRowCodableRoundTrip() throws {
        let row = PluginRow(id: "com.maydow.weather-pro", name: "Weather Pro", version: "1.0.0", enabled: true, attestState: "good")
        let data = try JSONEncoder().encode(row)
        let decoded = try JSONDecoder().decode(PluginRow.self, from: data)
        XCTAssertEqual(decoded, row)
    }
}
