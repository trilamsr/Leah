import Foundation
import SwiftUI

// Phase 4 §7.11: schema-only widget renderers. The host hands the SDK a typed
// PluginWidgetSchema decoded from the plugin's emitted JSON; SwiftUI renders the
// fields with no plugin-supplied code so a malicious plugin cannot inject views.

/// One field a plugin widget projects onto the HUD. `kind` constrains what the SwiftUI
/// renderer is willing to draw — anything else falls through to a `.string` formatter.
public struct PluginWidgetField: Codable, Equatable, Hashable {
    public enum Kind: String, Codable {
        case number
        case string
        case bool
    }

    public let id: String
    public let kind: Kind
    public let unit: String?

    public init(id: String, kind: Kind, unit: String? = nil) {
        self.id = id
        self.kind = kind
        self.unit = unit
    }
}

/// Decoded form of WidgetSchema.Schema emitted by the Go SDK.
public struct PluginWidgetSchema: Codable, Equatable {
    public let fields: [PluginWidgetField]

    public init(fields: [PluginWidgetField]) {
        self.fields = fields
    }

    /// Decode the raw JSON the daemon forwarded from the plugin's `EmitWidget` call.
    /// Failure returns nil so the host can mark the widget invalid without crashing.
    public static func decode(_ data: Data) -> PluginWidgetSchema? {
        try? JSONDecoder().decode(PluginWidgetSchema.self, from: data)
    }
}

/// One key/value the daemon delivers per widget update tick — keyed by the field id
/// the plugin declared in its schema.
public struct PluginWidgetValue: Equatable {
    public let fieldId: String
    public let display: String

    public init(fieldId: String, display: String) {
        self.fieldId = fieldId
        self.display = display
    }
}

/// Renderer-only SwiftUI surface — never accepts plugin-supplied views, only declared field ids
/// and primitive values formatted in-process.
public struct PluginWidget: View {
    public let title: String
    public let schema: PluginWidgetSchema
    public let values: [String: PluginWidgetValue]

    public init(title: String, schema: PluginWidgetSchema, values: [String: PluginWidgetValue]) {
        self.title = title
        self.schema = schema
        self.values = values
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title)
                .font(.system(size: 11, weight: .semibold))
                .foregroundColor(.secondary)
            ForEach(schema.fields, id: \.id) { field in
                HStack {
                    Text(field.id)
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                    Spacer()
                    Text(Self.format(field: field, value: values[field.id]))
                        .font(.system(size: 13, weight: .medium))
                        .monospacedDigit()
                }
            }
        }
        .padding(10)
        .background(RoundedRectangle(cornerRadius: 8).fill(Color.black.opacity(0.25)))
    }

    /// Centralized so unit append + missing-value handling stay consistent across renderers.
    public static func format(field: PluginWidgetField, value: PluginWidgetValue?) -> String {
        guard let v = value else { return "—" }
        if let unit = field.unit, !unit.isEmpty {
            return "\(v.display)\(unit)"
        }
        return v.display
    }
}
