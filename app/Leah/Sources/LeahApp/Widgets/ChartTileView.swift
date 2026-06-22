import SwiftUI
import Charts

struct ChartTilePoint: Identifiable {
  let id: Int
  let series: String
  let x: Double
  let y: Double
  let isAccent: Bool
}

struct ChartTileEvent: Identifiable {
  let id: Int
  let x: Double
  let y: Double
  let label: String
}

public struct ChartTileView: View {
  let envelope: WidgetEnvelope

  public init(envelope: WidgetEnvelope) {
    self.envelope = envelope
  }

  private var kind: String { (envelope.props["kind"]?.value as? String) ?? "line" }
  private var accentName: String { (envelope.props["accent"]?.value as? String) ?? firstSeriesName }

  private var firstSeriesName: String {
    guard let arr = envelope.props["series"]?.value as? [AnyCodable],
          let first = arr.first?.value as? [String: AnyCodable],
          let name = first["name"]?.value as? String else { return "" }
    return name
  }

  private var points: [ChartTilePoint] {
    guard let arr = envelope.props["series"]?.value as? [AnyCodable] else { return [] }
    var out: [ChartTilePoint] = []
    var idCounter = 0
    for s in arr {
      guard let obj = s.value as? [String: AnyCodable],
            let name = obj["name"]?.value as? String,
            let pts = obj["points"]?.value as? [AnyCodable] else { continue }
      let isAccent = name == accentName
      for p in pts {
        guard let pobj = p.value as? [String: AnyCodable],
              let x = numeric(pobj["x"]),
              let y = numeric(pobj["y"]) else { continue }
        out.append(ChartTilePoint(id: idCounter, series: name, x: x, y: y, isAccent: isAccent))
        idCounter += 1
      }
    }
    return out
  }

  private var events: [ChartTileEvent] {
    guard let arr = envelope.props["events"]?.value as? [AnyCodable] else { return [] }
    var out: [ChartTileEvent] = []
    for (i, e) in arr.enumerated() {
      guard let obj = e.value as? [String: AnyCodable],
            let x = numeric(obj["x"]),
            let y = numeric(obj["y"]) else { continue }
      let label = (obj["label"]?.value as? String) ?? ""
      out.append(ChartTileEvent(id: i, x: x, y: y, label: label))
    }
    return out
  }

  public var body: some View {
    Chart {
      ForEach(points) { p in
        seriesMark(for: p)
      }
      ForEach(events) { e in
        PointMark(x: .value("x", e.x), y: .value("y", e.y))
          .symbol(.diamond)
          .symbolSize(64)
          .foregroundStyle(Color.leahRedAlert)
      }
    }
    .chartXAxis {
      AxisMarks { _ in
        AxisGridLine().foregroundStyle(Color.leahIvory.opacity(0.2))
        AxisTick().foregroundStyle(Color.leahIvory.opacity(0.4))
      }
    }
    .chartYAxis {
      AxisMarks { _ in
        AxisGridLine().foregroundStyle(Color.leahIvory.opacity(0.2))
        AxisTick().foregroundStyle(Color.leahIvory.opacity(0.4))
      }
    }
    .padding(12)
  }

  @ChartContentBuilder
  private func seriesMark(for p: ChartTilePoint) -> some ChartContent {
    // Direct foregroundStyle only — `by: .value("series", …)` collapses to the
    // chart's default categorical palette and discards explicit gold/ivory.
    let color: Color = p.isAccent ? .leahGold : .leahIvory.opacity(0.4)
    switch kind {
    case "bar":
      BarMark(x: .value("x", p.x), y: .value("y", p.y))
        .foregroundStyle(color)
    case "area":
      AreaMark(x: .value("x", p.x), y: .value("y", p.y))
        .foregroundStyle(color.opacity(p.isAccent ? 0.6 : 0.25))
    case "scatter":
      PointMark(x: .value("x", p.x), y: .value("y", p.y))
        .foregroundStyle(color)
        .symbolSize(36)
    default: // "line" and "sparkline"
      LineMark(x: .value("x", p.x),
               y: .value("y", p.y),
               series: .value("series", p.series))
        .foregroundStyle(color)
        .lineStyle(StrokeStyle(lineWidth: p.isAccent ? 2 : 1))
    }
  }

  private func numeric(_ a: AnyCodable?) -> Double? {
    guard let v = a?.value else { return nil }
    if let d = v as? Double { return d }
    if let i = v as? Int { return Double(i) }
    return nil
  }
}
