import SwiftUI

public struct StatWidget: LeahWidget {
    public let label: String
    public let value: String
    public let trend: String?
    public let size: WidgetSize

    public init(label: String, value: String, trend: String?, size: WidgetSize = .small) {
        self.label = label
        self.value = value
        self.trend = trend
        self.size = size
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(label)
                .font(.caption.weight(.medium))
                .tracking(0.5)
                .foregroundColor(Palette.textMuted)
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(value)
                    .font(.system(.title, design: .monospaced).weight(.medium))
                    .foregroundColor(Palette.ivory)
                if let t = trend {
                    Text(t)
                        .font(.system(.callout, design: .monospaced))
                        .foregroundColor(trendColor(t))
                }
            }
        }
        .padding(16)
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(Palette.hairline, lineWidth: 1)
        )
    }

    private func trendColor(_ t: String) -> Color {
        t.hasPrefix("-") ? Palette.oxblood : Palette.accentGold
    }
}
