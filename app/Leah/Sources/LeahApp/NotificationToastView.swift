import SwiftUI

public struct Toast: Identifiable, Equatable {
  public enum Severity: String, Equatable { case info, warn, redAlert }

  public let id: String
  public let kind: String
  public let title: String
  public let body: String
  public let actionLabel: String?
  public let severity: Severity
  public let createdAt: Date

  public init(id: String, kind: String, title: String, body: String, actionLabel: String?, severity: Severity, createdAt: Date) {
    self.id = id
    self.kind = kind
    self.title = title
    self.body = body
    self.actionLabel = actionLabel
    self.severity = severity
    self.createdAt = createdAt
  }
}

public struct NotificationToastView: View {
  public let toast: Toast
  public let onAcknowledge: (Toast) -> Void
  public let onExpand: (Toast) -> Void

  public init(toast: Toast, onAcknowledge: @escaping (Toast) -> Void, onExpand: @escaping (Toast) -> Void) {
    self.toast = toast
    self.onAcknowledge = onAcknowledge
    self.onExpand = onExpand
  }

  public static func height(for toast: Toast) -> CGFloat {
    toast.actionLabel == nil ? 64 : 112
  }

  public func expand() { onExpand(toast) }
  public func acknowledge() { onAcknowledge(toast) }

  public var body: some View {
    VStack(alignment: .leading, spacing: 6) {
      HStack(spacing: 8) {
        Circle().fill(severityColor).frame(width: 6, height: 6)
        Text(toast.title).font(.callout.weight(.medium)).foregroundColor(LeahPalette.ivory)
        Spacer(minLength: 0)
        Text("Mark").font(.caption).foregroundColor(LeahPalette.textMuted)
      }
      if !toast.body.isEmpty {
        Text(toast.body).font(.caption).foregroundColor(LeahPalette.textMuted).lineLimit(1)
      }
      if let label = toast.actionLabel {
        HStack {
          Spacer(minLength: 0)
          Text(label)
            .font(.caption.weight(.medium))
            .foregroundColor(LeahPalette.champagneGold)
            .padding(.horizontal, 10).padding(.vertical, 4)
            .overlay(RoundedRectangle(cornerRadius: 6).stroke(LeahPalette.hairline, lineWidth: 1))
        }
      }
    }
    .padding(.horizontal, 12).padding(.vertical, 10)
    .frame(width: 320, height: Self.height(for: toast), alignment: .topLeading)
    .background(LeahPalette.obsidian2)
    .overlay(RoundedRectangle(cornerRadius: 10).stroke(LeahPalette.hairline, lineWidth: 1))
    .contentShape(Rectangle())
    .onTapGesture { expand() }
    .gesture(DragGesture(minimumDistance: 24).onEnded { v in
      if v.translation.width > 24 { acknowledge() }
    })
  }

  private var severityColor: Color {
    switch toast.severity {
    case .info: return LeahPalette.champagneGold
    case .warn: return LeahPalette.champagneGold
    case .redAlert: return Color.leahRedAlert
    }
  }
}
