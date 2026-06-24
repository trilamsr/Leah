import SwiftUI

// §9.0: ambient stack of 3 pinned + 3 HUD + 1-3 toast cards = 600px column at
// 1440×900 (>60% screen height), breaking the <2% screen-area promise. Hard
// cap pinned at 2 in ambient; remainder collapses into a single disclosure
// row that routes to Dashboard (the only surface allowed to render the full
// pin list).
public enum PinnedWidgetStackPolicy {
  public static let maxPinnedInAmbient = 2
}

public struct PinnedWidgetStack: View {
  public let pinnedWidgets: [WidgetEnvelope]
  public let tile: (WidgetEnvelope) -> AnyView
  public let onOverflowTap: () -> Void

  public init(
    pinnedWidgets: [WidgetEnvelope],
    tile: @escaping (WidgetEnvelope) -> AnyView,
    onOverflowTap: @escaping () -> Void = PinnedWidgetStack.defaultOverflowTap
  ) {
    self.pinnedWidgets = pinnedWidgets
    self.tile = tile
    self.onOverflowTap = onOverflowTap
  }

  public static func defaultOverflowTap() {
    NotificationCenter.default.post(name: .leahOpenDashboard, object: nil)
  }

  public var visible: [WidgetEnvelope] {
    Array(pinnedWidgets.prefix(PinnedWidgetStackPolicy.maxPinnedInAmbient))
  }
  public var overflowCount: Int {
    max(0, pinnedWidgets.count - PinnedWidgetStackPolicy.maxPinnedInAmbient)
  }
  public var hasOverflow: Bool { overflowCount > 0 }

  public var body: some View {
    VStack(alignment: .trailing, spacing: 8) {
      ForEach(visible, id: \.id) { env in
        tile(env)
      }
      if hasOverflow {
        overflowRow
      }
    }
  }

  private var overflowRow: some View {
    HStack {
      Text("+\(overflowCount) more pinned")
        .font(.system(size: 12))
        .foregroundColor(LeahPalette.textMuted)
      Spacer(minLength: 0)
    }
    .padding(.horizontal, 12).padding(.vertical, 8)
    .frame(width: 320)
    .background(LeahPalette.obsidian2)
    .overlay(RoundedRectangle(cornerRadius: 10).stroke(LeahPalette.hairline, lineWidth: 1))
    .contentShape(Rectangle())
    .onTapGesture { onOverflowTap() }
    .accessibilityLabel("Show \(overflowCount) more pinned widgets in Dashboard")
  }
}
