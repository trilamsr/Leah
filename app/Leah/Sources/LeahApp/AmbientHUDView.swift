import SwiftUI

public struct AmbientHUDData: Equatable {
  public let nextEventTitle: String?
  public let briefInProgressTitle: String?
  public let firstUncompletedAgenda: String?
  public let prCount: Int
  public let briefCount: Int
  public let arxivCount: Int

  public init(
    nextEventTitle: String?,
    briefInProgressTitle: String?,
    firstUncompletedAgenda: String?,
    prCount: Int,
    briefCount: Int,
    arxivCount: Int
  ) {
    self.nextEventTitle = nextEventTitle
    self.briefInProgressTitle = briefInProgressTitle
    self.firstUncompletedAgenda = firstUncompletedAgenda
    self.prCount = prCount
    self.briefCount = briefCount
    self.arxivCount = arxivCount
  }

  public static let empty = AmbientHUDData(
    nextEventTitle: nil,
    briefInProgressTitle: nil,
    firstUncompletedAgenda: nil,
    prCount: 0,
    briefCount: 0,
    arxivCount: 0
  )
}

public struct AmbientHUDView: View {
  public static let size = CGSize(width: 280, height: 84)
  /// §10 canvas invariant — Mark glyph is the only gold region on the HUD.
  public static let goldRegionCount: Int = 1
  /// §3.3 reserves Tiempos / New York Italic for one dashboard moment only.
  public static let fontFamilies: [String] = ["Inter", "SF Pro Text"]

  public let clock: () -> Date
  public let data: AmbientHUDData
  @State private var pulseRotation: Int = 0

  public init(clock: @escaping () -> Date = Date.init, data: AmbientHUDData) {
    self.clock = clock
    self.data = data
  }

  public func greeting() -> String {
    let h = Calendar(identifier: .gregorian).component(.hour, from: clock())
    switch h {
    case 6..<11: return "Good morning, Tri."
    case 11..<17: return "Good afternoon, Tri."
    default: return "Good evening, Tri."
    }
  }

  public func nowRowText() -> String? {
    let h = Calendar(identifier: .gregorian).component(.hour, from: clock())
    switch h {
    case 8..<12: return data.nextEventTitle
    case 12..<18: return data.briefInProgressTitle
    default: return data.firstUncompletedAgenda
    }
  }

  public func pulseText(rotation: Int) -> String {
    switch rotation % 3 {
    case 0: return "\u{25C7} \(data.prCount) PRs"
    case 1: return "\u{232C} \(data.briefCount) briefs"
    default: return "\u{2387} \(data.arxivCount) arxiv"
    }
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 6) {
      HStack(spacing: 8) {
        MarkGlyph().frame(width: 24, height: 24)
        Text(greeting())
          .font(.caption.weight(.medium))
          .foregroundColor(LeahPalette.textMuted)
        Spacer(minLength: 0)
      }
      if let now = nowRowText() {
        Text(now)
          .font(.callout)
          .foregroundColor(LeahPalette.ivory)
          .lineLimit(1)
          .truncationMode(.tail)
      }
      Spacer(minLength: 0)
      Text(pulseText(rotation: pulseRotation))
        .font(.system(.caption, design: .monospaced))
        .foregroundColor(LeahPalette.textMuted)
    }
    .padding(.horizontal, 14)
    .padding(.vertical, 10)
    .frame(width: Self.size.width, height: Self.size.height, alignment: .topLeading)
    .background(Color(red: 0x0E/255, green: 0x10/255, blue: 0x14/255))
    .onHover { inside in
      if inside { pulseRotation = (pulseRotation + 1) % 3 }
    }
  }
}

private struct MarkGlyph: View {
  var body: some View {
    Canvas { ctx, size in
      let r = min(size.width, size.height) / 2 - 1
      let c = CGPoint(x: size.width / 2, y: size.height / 2)
      var path = Path()
      for i in 0..<6 {
        let a = Double(i) * .pi / 3 - .pi / 2
        let p = CGPoint(x: c.x + r * cos(a), y: c.y + r * sin(a))
        if i == 0 { path.move(to: p) } else { path.addLine(to: p) }
      }
      path.closeSubpath()
      ctx.stroke(path, with: .color(LeahPalette.champagneGold), lineWidth: 1.0)
    }
  }
}
