import SwiftUI

// Unified-diff tile. Spec §10.1 item 13.
// +lines = champagne-gold left hairline; −lines = red-alert (#C8434F) — matches
// gutter-glyph color, distinct from line-bg oxblood (#7A1F2B).
// Hunk headers in JetBrains Mono, tracking +0.04em (≈ 0.52pt @ 13pt).

public struct DiffTilePayload {
  public enum Error: Swift.Error, Equatable {
    case unifiedDiffRequired
  }

  public enum LineKind: Equatable {
    case context, added, removed, hunkHeader

    public var hairlineColor: Color? {
      switch self {
      case .added:    return CodeDiffPalette.champagneGold
      case .removed:  return CodeDiffPalette.redAlert
      default:        return nil
      }
    }
  }

  public struct Line: Equatable {
    public let kind: LineKind
    public let text: String
    public var hairlineColor: Color? { kind.hairlineColor }
  }

  // +0.04em at 13pt body sans = 0.52pt.
  public static let hunkHeaderTracking: CGFloat = 0.52

  public let filename: String?
  public let lines: [Line]

  public init(envelope: WidgetEnvelope) throws {
    guard let unified = envelope.props["unified"]?.value as? String, !unified.isEmpty else {
      throw Error.unifiedDiffRequired
    }
    self.filename = envelope.props["filename"]?.value as? String
    self.lines = Self.parse(unified: unified)
  }

  static func parse(unified: String) -> [Line] {
    unified.split(separator: "\n", omittingEmptySubsequences: false).compactMap { raw -> Line? in
      let s = String(raw)
      if s.isEmpty { return nil }
      if s.hasPrefix("@@") { return Line(kind: .hunkHeader, text: s) }
      if s.hasPrefix("+") { return Line(kind: .added, text: String(s.dropFirst())) }
      if s.hasPrefix("-") { return Line(kind: .removed, text: String(s.dropFirst())) }
      if s.hasPrefix(" ") { return Line(kind: .context, text: String(s.dropFirst())) }
      return Line(kind: .context, text: s)
    }
  }
}

public struct DiffTileView: View {
  let payload: DiffTilePayload

  public init(payload: DiffTilePayload) { self.payload = payload }

  public var body: some View {
    VStack(alignment: .leading, spacing: 0) {
      header
      Rectangle().fill(CodeDiffPalette.hairline).frame(height: 1)
      ScrollView(.vertical) {
        VStack(alignment: .leading, spacing: 1) {
          ForEach(Array(payload.lines.enumerated()), id: \.offset) { _, line in
            row(line)
          }
        }
        .padding(.vertical, 12)
      }
    }
    .background(CodeDiffPalette.obsidian1)
    .cornerRadius(12)
    .overlay(
      RoundedRectangle(cornerRadius: 12).strokeBorder(CodeDiffPalette.hairline, lineWidth: 1)
    )
  }

  private var header: some View {
    HStack {
      Text("DIFF")
        .font(.caption.weight(.medium))
        .tracking(DiffTilePayload.hunkHeaderTracking)
        .foregroundColor(CodeDiffPalette.textMuted)
      if let f = payload.filename {
        Text("· \(f)")
          .font(.caption)
          .tracking(DiffTilePayload.hunkHeaderTracking)
          .foregroundColor(CodeDiffPalette.textMuted)
      }
      Spacer()
    }
    .padding(.horizontal, 16).padding(.vertical, 10)
  }

  @ViewBuilder
  private func row(_ line: DiffTilePayload.Line) -> some View {
    HStack(spacing: 0) {
      Rectangle()
        .fill(line.hairlineColor ?? Color.clear)
        .frame(width: 1.5)
      switch line.kind {
      case .hunkHeader:
        Text(line.text)
          .font(.system(.caption, design: .monospaced).weight(.medium))
          .tracking(DiffTilePayload.hunkHeaderTracking)
          .foregroundColor(CodeDiffPalette.textMuted)
          .padding(.leading, 8)
      default:
        Text(prefix(for: line.kind) + line.text)
          .font(.system(.callout, design: .monospaced))
          .foregroundColor(textColor(for: line.kind))
          .padding(.leading, 8)
      }
      Spacer(minLength: 0)
    }
    .padding(.horizontal, 12)
    .accessibilityElement(children: .combine)
    .accessibilityLabel(accessibilityLabel(for: line))
  }

  private func accessibilityLabel(for line: DiffTilePayload.Line) -> String {
    switch line.kind {
    case .added:      return "added \(line.text)"
    case .removed:    return "removed \(line.text)"
    case .context:    return "context \(line.text)"
    case .hunkHeader: return "hunk header \(line.text)"
    }
  }

  private func prefix(for kind: DiffTilePayload.LineKind) -> String {
    switch kind {
    case .added:   return "+ "
    case .removed: return "- "
    case .context: return "  "
    case .hunkHeader: return ""
    }
  }

  private func textColor(for kind: DiffTilePayload.LineKind) -> Color {
    switch kind {
    case .added:   return CodeDiffPalette.ivory
    case .removed: return CodeDiffPalette.ivory
    case .context: return CodeDiffPalette.textMuted
    case .hunkHeader: return CodeDiffPalette.textMuted
    }
  }
}
