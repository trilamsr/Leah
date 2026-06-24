import SwiftUI
import Charts

struct MarketTileSeriesPoint: Identifiable {
  let id: Int
  let y: Double
}

public struct MarketTileView: View {
  let envelope: WidgetEnvelope

  public init(envelope: WidgetEnvelope) {
    self.envelope = envelope
  }

  // Spec § 10.1 sparkline cap: ≤30 points per tile (perf + density).
  private static let sparklineMaxPoints = 30

  private var symbol: String { stringProp("symbol") ?? "—" }
  private var price: String { stringProp("price") ?? "————" }
  private var changeText: String { stringProp("change_pct") ?? stringProp("change") ?? "" }

  private var delta: Double {
    if let d = doubleProp("delta") { return d }
    if let c = doubleProp("change") { return c }
    return 0
  }

  private var sparkline: [MarketTileSeriesPoint] {
    guard let raw = envelope.props["sparkline"]?.value as? [AnyCodable] else { return [] }
    let nums = raw.compactMap { a -> Double? in
      if let d = a.value as? Double { return d }
      if let i = a.value as? Int { return Double(i) }
      return nil
    }
    let trimmed = nums.suffix(Self.sparklineMaxPoints)
    return trimmed.enumerated().map { MarketTileSeriesPoint(id: $0.offset, y: $0.element) }
  }

  // Spec § 10.1 line 1033: "Positive delta = ivory + ▲. No green, ever."
  // Negative delta = oxblood. Gold lives on the symbol hairline only.
  private var deltaColor: Color { delta < 0 ? .leahRedAlert : .leahIvory }
  private var deltaGlyph: String { delta < 0 ? "▼" : "▲" }

  public var body: some View {
    HStack(alignment: .center, spacing: 12) {
      VStack(alignment: .leading, spacing: 4) {
        Text(symbol)
          .font(.system(.callout, design: .monospaced).weight(.medium))
          .foregroundColor(.leahIvory)
        // Single gold hairline anchors the symbol; one connected region toward
        // the 3-instance budget vs. N letter glyphs if the symbol were tinted.
        Rectangle()
          .fill(Color.leahGold)
          .frame(width: 28, height: 1.5)
        Text(price)
          .font(.system(.title2, design: .monospaced).weight(.semibold))
          .foregroundColor(.leahIvory)
        HStack(spacing: 4) {
          Text(deltaGlyph)
            .foregroundColor(deltaColor)
          Text(changeText)
            .foregroundColor(.leahIvory)
        }
        .font(.system(.caption, design: .monospaced))
      }
      Spacer(minLength: 8)
      sparklineChart
        .frame(width: 120, height: 48)
    }
    .padding(12)
    .background(Color.black.opacity(0.001))
  }

  @ViewBuilder
  private var sparklineChart: some View {
    if sparkline.isEmpty {
      EmptyView()
    } else {
      Chart(sparkline) { p in
        LineMark(x: .value("i", p.id), y: .value("y", p.y))
          .foregroundStyle(Color.leahGold.opacity(0.6))
          .lineStyle(StrokeStyle(lineWidth: 2))
      }
      .chartXAxis(.hidden)
      .chartYAxis(.hidden)
      .chartPlotStyle { $0.background(Color.clear) }
    }
  }

  private func stringProp(_ key: String) -> String? {
    guard let v = envelope.props[key]?.value else { return nil }
    if let s = v as? String { return s }
    if let d = v as? Double { return String(d) }
    if let i = v as? Int { return String(i) }
    return nil
  }

  private func doubleProp(_ key: String) -> Double? {
    guard let v = envelope.props[key]?.value else { return nil }
    if let d = v as? Double { return d }
    if let i = v as? Int { return Double(i) }
    if let s = v as? String, let d = Double(s) { return d }
    return nil
  }
}
