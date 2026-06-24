import SwiftUI

public struct FlightsCell: Codable, Equatable {
  public let date: String
  public let priceCents: Int
  public let carrier: String?
  public let stops: Int
  public let globalMin: Bool

  enum CodingKeys: String, CodingKey {
    case date, carrier, stops
    case priceCents = "price_cents"
    case globalMin = "global_min"
  }

  public init(date: String, priceCents: Int, carrier: String? = nil, stops: Int = 0, globalMin: Bool = false) {
    self.date = date; self.priceCents = priceCents
    self.carrier = carrier; self.stops = stops; self.globalMin = globalMin
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    self.date = try c.decode(String.self, forKey: .date)
    self.priceCents = try c.decode(Int.self, forKey: .priceCents)
    self.carrier = try c.decodeIfPresent(String.self, forKey: .carrier)
    self.stops = try c.decodeIfPresent(Int.self, forKey: .stops) ?? 0
    self.globalMin = try c.decodeIfPresent(Bool.self, forKey: .globalMin) ?? false
  }
}

public struct FlightsPayload: Codable, Equatable {
  public let origin: String
  public let destination: String
  public let currency: String
  public let cells: [FlightsCell]

  enum CodingKeys: String, CodingKey { case origin, destination, currency, cells }
}

public enum FlightsCellStyle: Equatable {
  case globalMin
  case rowMin
  case high
  case normal

  public var background: String? {
    self == .globalMin ? FlightsTileStyle.goldHex : nil
  }
  public var foreground: String {
    switch self {
    case .globalMin: return FlightsTileStyle.obsidianHex
    case .high:      return FlightsTileStyle.oxbloodHex
    default:         return FlightsTileStyle.ivoryHex
    }
  }
  public var leadingHairlineWidth: CGFloat {
    self == .rowMin ? 1.5 : 0
  }
  public var leadingHairlineHex: String? {
    self == .rowMin ? FlightsTileStyle.goldHex : nil
  }
}

public enum FlightsTileStyle {
  public static let goldHex = "#C9A961"
  public static let obsidianHex = "#08090C"
  public static let ivoryHex = "#F2EDE0"
  public static let oxbloodHex = "#D75A66"

  public static let maxCells = 28
  public static let highFareMultiplier = 1.30
}

public struct StyledFlightsCell: Equatable {
  public let cell: FlightsCell
  public let style: FlightsCellStyle
}

public struct FlightsTileModel {
  public let payload: FlightsPayload
  public let styledCells: [StyledFlightsCell]
  public let rowMedianByDate: [String: Int]

  public init(payload: FlightsPayload) {
    self.payload = payload
    let capped = Array(payload.cells.prefix(FlightsTileStyle.maxCells))

    let globalMinIdx = capped.enumerated().min(by: { $0.element.priceCents < $1.element.priceCents })?.offset
    let prices = capped.map(\.priceCents).sorted()
    let median: Int = prices.isEmpty
      ? 0
      : (prices.count % 2 == 0
         ? (prices[prices.count/2 - 1] + prices[prices.count/2]) / 2
         : prices[prices.count/2])

    // Single-row matrix (one origin→destination per fixture day-strip).
    // Row median = full set median for v1; multi-row layouts will pass an
    // explicit row key when added in Wave 3.
    var medians: [String: Int] = [:]
    for c in capped { medians[c.date] = median }
    self.rowMedianByDate = medians

    // Lowest non-global-min = row-min hairline (single row).
    let nonGlobalSorted = capped.enumerated()
      .filter { $0.offset != globalMinIdx }
      .sorted { $0.element.priceCents < $1.element.priceCents }
    let rowMinIdx = nonGlobalSorted.first?.offset

    self.styledCells = capped.enumerated().map { (i, c) in
      let s: FlightsCellStyle
      if i == globalMinIdx { s = .globalMin }
      else if i == rowMinIdx { s = .rowMin }
      else if Double(c.priceCents) >= Double(median) * FlightsTileStyle.highFareMultiplier { s = .high }
      else { s = .normal }
      return StyledFlightsCell(cell: c, style: s)
    }
  }

  public func rowMedianCents(for cell: FlightsCell) -> Int {
    rowMedianByDate[cell.date] ?? 0
  }

  // 1 global-min fill + 1 row-min hairline + 1 route label = 3 gold regions max.
  public var goldRegionCount: Int {
    let gm = styledCells.contains { $0.style == .globalMin } ? 1 : 0
    let rm = styledCells.contains { $0.style == .rowMin } ? 1 : 0
    return gm + rm + (routeLabelUsesGold ? 1 : 0)
  }

  public var routeLabelUsesGold: Bool { true }
}

public struct FlightsTileView: View {
  public let payload: FlightsPayload
  private let model: FlightsTileModel

  public init(payload: FlightsPayload) {
    self.payload = payload
    self.model = FlightsTileModel(payload: payload)
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 8) {
      eyebrow
      grid
    }
    .padding(12)
  }

  private var eyebrow: some View {
    Text("FLIGHTS · \(payload.origin) → \(payload.destination)")
      .font(.system(.caption, design: .default).weight(.regular))
      .tracking(0.04 * 11)
      .foregroundColor(Color(hex: FlightsTileStyle.goldHex))
  }

  private var grid: some View {
    let columns = [GridItem(.adaptive(minimum: 64), spacing: 6)]
    return LazyVGrid(columns: columns, alignment: .leading, spacing: 6) {
      ForEach(Array(model.styledCells.enumerated()), id: \.offset) { _, sc in
        cellView(sc)
      }
    }
  }

  private func cellView(_ sc: StyledFlightsCell) -> some View {
    let bg: Color = sc.style.background.map { Color(hex: $0) } ?? .clear
    let fg = Color(hex: sc.style.foreground)
    return HStack(spacing: 0) {
      if sc.style.leadingHairlineWidth > 0, let hex = sc.style.leadingHairlineHex {
        Rectangle()
          .fill(Color(hex: hex))
          .frame(width: sc.style.leadingHairlineWidth)
      }
      VStack(alignment: .leading, spacing: 2) {
        Text(sc.cell.date.suffix(5).replacingOccurrences(of: "-", with: "/"))
          .font(.system(.caption2, design: .monospaced))
          .foregroundColor(fg.opacity(0.7))
        Text(priceLabel(sc.cell.priceCents))
          .font(.system(.caption, design: .monospaced))
          .foregroundColor(fg)
      }
      .padding(.horizontal, 6)
      .padding(.vertical, 4)
    }
    .background(bg)
  }

  private func priceLabel(_ cents: Int) -> String {
    "$\(cents / 100)"
  }
}

extension Color {
  init(hex: String) {
    var s = hex
    if s.hasPrefix("#") { s.removeFirst() }
    var v: UInt64 = 0
    Scanner(string: s).scanHexInt64(&v)
    let r = Double((v >> 16) & 0xff) / 255.0
    let g = Double((v >> 8) & 0xff) / 255.0
    let b = Double(v & 0xff) / 255.0
    self.init(.sRGB, red: r, green: g, blue: b, opacity: 1)
  }
}
