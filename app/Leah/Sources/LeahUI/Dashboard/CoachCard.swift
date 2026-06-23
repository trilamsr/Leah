import SwiftUI
import AppKit

public struct CoachStats: Equatable, Sendable {
  public let surfaced: Int
  public let dismissed: Int
  public let applied: Int

  public init(surfaced: Int, dismissed: Int, applied: Int) {
    self.surfaced = surfaced
    self.dismissed = dismissed
    self.applied = applied
  }

  public static let zero = CoachStats(surfaced: 0, dismissed: 0, applied: 0)
}

public struct CoachCard: View {
  public enum StatSlot: String, CaseIterable, Hashable, Sendable {
    case surfaced, dismissed, applied
  }

  public static let statSlots: [StatSlot] = [.surfaced, .dismissed, .applied]
  public static let tapTargetPane = "recommendations"

  public static func value(_ stats: CoachStats, for slot: StatSlot) -> Int {
    switch slot {
    case .surfaced:  return stats.surfaced
    case .dismissed: return stats.dismissed
    case .applied:   return stats.applied
    }
  }

  private let stats: CoachStats

  public init(stats: CoachStats) {
    self.stats = stats
  }

  public var body: some View {
    DashboardCardChrome(title: "Coach", tapTargetPane: Self.tapTargetPane) {
      HStack(alignment: .top, spacing: 24) {
        ForEach(Self.statSlots, id: \.self) { slot in
          stat(slot, Self.value(stats, for: slot))
        }
      }
    }
  }

  @ViewBuilder
  private func stat(_ slot: StatSlot, _ count: Int) -> some View {
    VStack(alignment: .leading, spacing: 6) {
      Text("\(count)")
        .font(.system(size: 28, weight: .semibold))
        .foregroundColor(DashboardPalette.ivory)
      Text(slot.rawValue)
        .font(.system(size: 11, weight: .medium))
        .foregroundColor(DashboardPalette.ivory.opacity(0.6))
        .textCase(.uppercase)
    }
  }
}
