import SwiftUI
import Foundation

@MainActor
public final class NotificationToastStack: ObservableObject {
  public static let visibleCap = 2

  @Published public private(set) var toasts: [Toast] = []
  private var sweepTimer: Timer?
  private let now: () -> Date

  public init(now: @escaping () -> Date = Date.init) {
    self.now = now
  }

  public var visible: [Toast] { Array(toasts.prefix(Self.visibleCap)) }
  public var overflowCount: Int { max(0, toasts.count - Self.visibleCap) }
  public var hasOverflow: Bool { overflowCount > 0 }
  public var activeTimerCount: Int { sweepTimer == nil ? 0 : 1 }

  public func push(_ toast: Toast) {
    if let i = toasts.firstIndex(where: { $0.kind == toast.kind && $0.title == toast.title }) {
      toasts[i] = toast
    } else {
      toasts.append(toast)
    }
    armSweep()
  }

  public func acknowledge(id: String) {
    toasts.removeAll { $0.id == id }
    if toasts.isEmpty { disarmSweep() }
  }

  // Single coalesced sweep — spec perf #35: one timer for N toasts, not N timers.
  public func sweepExpired(defaultLifetime: TimeInterval = 8) {
    let t = now()
    toasts.removeAll { tst in
      tst.severity != .redAlert && t.timeIntervalSince(tst.createdAt) >= defaultLifetime
    }
    if toasts.isEmpty { disarmSweep() }
  }

  private func armSweep() {
    guard sweepTimer == nil else { return }
    sweepTimer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: true) { [weak self] _ in
      Task { @MainActor in self?.sweepExpired() }
    }
  }

  private func disarmSweep() {
    sweepTimer?.invalidate()
    sweepTimer = nil
  }

  deinit { sweepTimer?.invalidate() }
}

public struct NotificationToastStackView: View {
  @ObservedObject public var stack: NotificationToastStack
  public let onExpand: (Toast) -> Void
  @State private var overflowExpanded = false

  public init(stack: NotificationToastStack, onExpand: @escaping (Toast) -> Void) {
    self.stack = stack
    self.onExpand = onExpand
  }

  public var body: some View {
    VStack(alignment: .trailing, spacing: 8) {
      ForEach(stack.visible) { t in
        NotificationToastView(
          toast: t,
          onAcknowledge: { stack.acknowledge(id: $0.id) },
          onExpand: onExpand
        )
        .transition(.move(edge: .top).combined(with: .opacity))
      }
      if stack.hasOverflow {
        overflowCard
      }
    }
    .animation(.easeOut(duration: 0.16), value: stack.toasts.map(\.id))
  }

  private var overflowCard: some View {
    HStack {
      Text("+\(stack.overflowCount) more").font(.caption).foregroundColor(LeahPalette.textMuted)
      Spacer(minLength: 0)
    }
    .padding(.horizontal, 12).padding(.vertical, 8)
    .frame(width: 320)
    .background(LeahPalette.obsidian2)
    .overlay(RoundedRectangle(cornerRadius: 10).stroke(LeahPalette.hairline, lineWidth: 1))
    .contentShape(Rectangle())
    .onTapGesture { overflowExpanded.toggle() }
  }
}
