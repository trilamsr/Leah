import SwiftUI
import AppKit

public enum HealthStatus: String, CaseIterable, Equatable, Sendable {
  case green, yellow, red
}

public struct ProcessHealth: Identifiable, Equatable, Sendable {
  public var id: String { name }
  public let name: String
  public let status: HealthStatus

  public init(name: String, status: HealthStatus) {
    self.name = name
    self.status = status
  }
}

public struct HealthCard: View {
  public static let tapTargetPane = "about.diagnostics"

  public static func rowCount(_ procs: [ProcessHealth]) -> Int { procs.count }

  private let processes: [ProcessHealth]

  public init(processes: [ProcessHealth]) {
    self.processes = processes
  }

  public var body: some View {
    DashboardCardChrome(title: "Health", tapTargetPane: Self.tapTargetPane) {
      VStack(alignment: .leading, spacing: 8) {
        ForEach(processes) { p in
          row(p)
        }
      }
    }
  }

  @ViewBuilder
  private func row(_ p: ProcessHealth) -> some View {
    HStack(spacing: 10) {
      Circle()
        .fill(color(for: p.status))
        .frame(width: 8, height: 8)
      Text(p.name)
        .font(.system(size: 12, weight: .medium))
        .foregroundColor(DashboardPalette.ivory.opacity(0.85))
      Spacer()
      Text(p.status.rawValue)
        .font(.system(size: 10, weight: .regular))
        .foregroundColor(DashboardPalette.ivory.opacity(0.5))
        .textCase(.uppercase)
    }
  }

  private func color(for status: HealthStatus) -> Color {
    switch status {
    case .green:  return Color(red: 0.36, green: 0.78, blue: 0.45)
    case .yellow: return Color(red: 0.95, green: 0.78, blue: 0.30)
    case .red:    return Color(red: 0.90, green: 0.36, blue: 0.36)
    }
  }
}

// Shared card chrome — title row + content + whole-card tap that routes to
// Settings via the leahOpenSettings notification (consumed by LeahApp).
struct DashboardCardChrome<Content: View>: View {
  let title: String
  let tapTargetPane: String
  @ViewBuilder let content: () -> Content

  var body: some View {
    Button(action: openPane) {
      VStack(alignment: .leading, spacing: 14) {
        Text(title)
          .font(.system(size: 13, weight: .semibold))
          .foregroundColor(DashboardPalette.ivory.opacity(0.55))
          .textCase(.uppercase)
        content()
      }
      .padding(20)
      .frame(maxWidth: .infinity, alignment: .topLeading)
      .contentShape(Rectangle())
    }
    .buttonStyle(.plain)
  }

  private func openPane() {
    NotificationCenter.default.post(
      name: .leahOpenSettings,
      object: nil,
      userInfo: [Notification.leahSettingsPaneKey: tapTargetPane]
    )
  }
}
