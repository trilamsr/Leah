import SwiftUI

public struct TableWidget: LeahWidget {
    public let headers: [String]
    public let rows: [[String]]
    public let negativeMask: [[Bool]]?
    public let size: WidgetSize

    public init(
        headers: [String],
        rows: [[String]],
        negativeMask: [[Bool]]? = nil,
        size: WidgetSize = .medium
    ) {
        self.headers = headers
        self.rows = rows
        self.negativeMask = negativeMask
        self.size = size
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 16) {
                ForEach(headers.indices, id: \.self) { i in
                    Text(headers[i])
                        .font(.caption.weight(.medium))
                        .tracking(0.5)
                        .foregroundColor(Palette.accentGold)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .padding(.vertical, 8)
            .padding(.horizontal, 12)

            Palette.hairline.frame(height: 1)

            ForEach(rows.indices, id: \.self) { r in
                HStack(spacing: 16) {
                    ForEach(rows[r].indices, id: \.self) { c in
                        Text(rows[r][c])
                            .font(.callout)
                            .foregroundColor(cellColor(row: r, col: c))
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
                .padding(.vertical, 6)
                .padding(.horizontal, 12)
            }
        }
        .background(Palette.obsidian2)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(Palette.hairline, lineWidth: 1)
        )
    }

    private func cellColor(row: Int, col: Int) -> Color {
        guard let mask = negativeMask,
              row < mask.count,
              col < mask[row].count,
              mask[row][col] else {
            return Palette.ivory
        }
        return Palette.oxblood
    }
}
