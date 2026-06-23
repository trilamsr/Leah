import XCTest
@testable import LeahPluginSDK

final class PluginWidgetTests: XCTestCase {
    func testDecodeWeatherSchema() {
        let raw = #"""
        {"fields":[{"id":"temperature_c","kind":"number","unit":"°C"},{"id":"location","kind":"string"}]}
        """#.data(using: .utf8)!
        let schema = PluginWidgetSchema.decode(raw)
        XCTAssertEqual(schema?.fields.count, 2)
        XCTAssertEqual(schema?.fields.first?.kind, .number)
        XCTAssertEqual(schema?.fields.first?.unit, "°C")
    }

    func testDecodeReturnsNilOnGarbage() {
        XCTAssertNil(PluginWidgetSchema.decode(Data("not json".utf8)))
    }

    func testFormatAppendsUnit() {
        let f = PluginWidgetField(id: "t", kind: .number, unit: "°C")
        let v = PluginWidgetValue(fieldId: "t", display: "18.3")
        XCTAssertEqual(PluginWidget.format(field: f, value: v), "18.3°C")
    }

    func testFormatRendersDashForMissing() {
        let f = PluginWidgetField(id: "loc", kind: .string)
        XCTAssertEqual(PluginWidget.format(field: f, value: nil), "—")
    }
}
