import SwiftUI

public struct CitationTileView: View {
  public enum Badge: String, Equatable { case stale, notFound }

  public let envelope: WidgetEnvelope

  // Body serif (not italic) — Tiempos/serif italic is Dashboard-only per spec §10.1+decision #28.
  public static let titleFont: Font = .system(.body, design: .serif)
  public static let goldSurfaceCount = 1

  public init(envelope: WidgetEnvelope) { self.envelope = envelope }

  public var body: some View {
    let p = envelope.props
    let site = (p["site_name"]?.value as? String) ?? ""
    let title = (p["title"]?.value as? String) ?? ""
    let snippet = Self.truncate((p["snippet"]?.value as? String) ?? "", max: 400)
    let badge = Self.badge(for: envelope)

    VStack(alignment: .leading, spacing: 6) {
      HStack(spacing: 6) {
        Text(site)
          .font(.system(size: 11, weight: .medium))
          .tracking(0.5)
          .foregroundColor(LeahPalette.champagneGold)
        if let b = badge { BadgeChip(badge: b) }
      }
      Text(title)
        .font(Self.titleFont)
        .foregroundColor(LeahPalette.ivory)
      if !snippet.isEmpty {
        Text(snippet)
          .font(.system(size: 12))
          .foregroundColor(LeahPalette.textMuted)
          .lineLimit(3)
      }
    }
    .padding(14)
    .background(LeahPalette.obsidian2)
    .cornerRadius(12)
    .overlay(
      RoundedRectangle(cornerRadius: 12)
        .strokeBorder(LeahPalette.hairline, lineWidth: 1)
    )
  }

  static func truncate(_ s: String, max: Int) -> String {
    s.count <= max ? s : String(s.prefix(max))
  }

  static func badge(for env: WidgetEnvelope) -> Badge? {
    if let code = env.props["status_code"]?.value as? Int, code == 404 { return .notFound }
    if let err = env.props["error"]?.value as? String, !err.isEmpty {
      return err.contains("404") ? .notFound : .stale
    }
    return nil
  }

  public static func register(in registry: WidgetTileRegistry) {
    registry.register(kind: .citation) { env in AnyView(CitationTileView(envelope: env)) }
  }
}

private struct BadgeChip: View {
  let badge: CitationTileView.Badge
  var body: some View {
    Text(badge == .stale ? "STALE" : "404")
      .font(.system(size: 10, weight: .semibold))
      .tracking(0.5)
      .foregroundColor(LeahPalette.ivory)
      .padding(.horizontal, 6)
      .padding(.vertical, 2)
      .background(LeahPalette.oxblood)
      .cornerRadius(3)
  }
}
