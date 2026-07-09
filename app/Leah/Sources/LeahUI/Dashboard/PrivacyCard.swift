import SwiftUI
import AppKit

public struct PrivacyBucketTrend: Identifiable, Equatable, Sendable {
  public let id: String          // stable bucket key, matches internal/budget.Bucket
  public let label: String       // short operator-visible label
  public let weekSpend: [Int64]  // 7 daily samples, oldest → newest

  public init(id: String, label: String, weekSpend: [Int64]) {
    self.id = id
    self.label = label
    self.weekSpend = weekSpend
  }
}

public struct PrivacyCard: View {
  public static let tapTargetPane = "privacy.budgets"

  // Mirrors internal/budget.Bucket enum order. Kept here as the operator-visible
  // surface — daemon IPC will replace placeholderTrends() once the wire is up.
  public static let bucketOrder: [PrivacyBucketTrend] = [
    PrivacyBucketTrend(id: "cloud.llm.tokens",     label: "Reasoning",     weekSpend: Array(repeating: 0, count: 7)),
    PrivacyBucketTrend(id: "cloud.embed.bytes",    label: "Embed",   weekSpend: Array(repeating: 0, count: 7)),
    PrivacyBucketTrend(id: "cloud.stt.seconds",    label: "STT",     weekSpend: Array(repeating: 0, count: 7)),
    PrivacyBucketTrend(id: "cloud.tts.chars",      label: "TTS",     weekSpend: Array(repeating: 0, count: 7)),
    PrivacyBucketTrend(id: "cloud.vision.bytes",   label: "Vision",  weekSpend: Array(repeating: 0, count: 7)),
    PrivacyBucketTrend(id: "peer.a2a.tokens",      label: "Peer",    weekSpend: Array(repeating: 0, count: 7)),
    PrivacyBucketTrend(id: "plugin.network.bytes", label: "Plugin",  weekSpend: Array(repeating: 0, count: 7)),
  ]

  public static func placeholderTrends() -> [PrivacyBucketTrend] {
    bucketOrder
  }

  private let trends: [PrivacyBucketTrend]

  public init(trends: [PrivacyBucketTrend]) {
    self.trends = trends
  }

  public var body: some View {
    DashboardCardChrome(title: "Privacy", tapTargetPane: Self.tapTargetPane) {
      VStack(alignment: .leading, spacing: 8) {
        ForEach(trends) { bucket in
          row(bucket)
        }
      }
    }
  }

  @ViewBuilder
  private func row(_ bucket: PrivacyBucketTrend) -> some View {
    HStack(spacing: 10) {
      Text(bucket.label)
        .font(.system(size: 11, weight: .medium))
        .foregroundColor(DashboardPalette.ivory.opacity(0.7))
        .frame(width: 56, alignment: .leading)
      sparkline(bucket.weekSpend)
        .frame(height: 14)
    }
  }

  @ViewBuilder
  private func sparkline(_ samples: [Int64]) -> some View {
    let peak = max(samples.max() ?? 1, 1)
    HStack(alignment: .bottom, spacing: 2) {
      ForEach(Array(samples.enumerated()), id: \.offset) { _, v in
        let h = max(2.0, CGFloat(Double(v) / Double(peak)) * 14.0)
        Rectangle()
          .fill(DashboardPalette.ivory.opacity(0.45))
          .frame(width: 6, height: h)
      }
    }
  }
}
