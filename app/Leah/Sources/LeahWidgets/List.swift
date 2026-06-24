import SwiftUI

public struct ListWidget: LeahWidget {
    public let items: [String]
    public let ordered: Bool
    public let size: WidgetSize

    public init(items: [String], ordered: Bool = false, size: WidgetSize = .small) {
        self.items = items
        self.ordered = ordered
        self.size = size
    }

    public var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 6) {
                ForEach(items.indices, id: \.self) { i in
                    HStack(alignment: .top, spacing: 8) {
                        Text(ordered ? "\(i + 1)." : "·")
                            .font(.callout)
                            .foregroundColor(Palette.accentGold)
                            .frame(minWidth: 16, alignment: .trailing)
                        Text(items[i])
                            .font(.callout)
                            .foregroundColor(Palette.ivory)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
            }
            .padding(16)
        }
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(Palette.hairline, lineWidth: 1)
        )
    }
}
