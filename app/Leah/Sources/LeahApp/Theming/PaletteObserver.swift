import Foundation
import AppKit
import SwiftUI
import Combine

public final class PaletteObserver: NSObject, ObservableObject {
  public static let crossfadeDuration: Double = 0.240

  @Published public private(set) var palette: Palette
  private var kvoToken: NSKeyValueObservation?
  private var subscribers: [UUID: (Palette) -> Void] = [:]
  private let reduceMotion: () -> Bool

  public init(initial: Palette.Mode,
              reduceMotion: @escaping () -> Bool = { NSWorkspace.shared.accessibilityDisplayShouldReduceMotion }) {
    self.palette = Palette.forMode(initial)
    self.reduceMotion = reduceMotion
    super.init()
  }

  // KVO on NSApp.effectiveAppearance — fires on system appearance toggle and on
  // manual override via Settings (which writes NSApp.appearance, propagating to effective).
  public func attach(to app: NSApplication = NSApp) {
    kvoToken = app.observe(\.effectiveAppearance, options: [.new]) { [weak self] app, _ in
      guard let self else { return }
      let mode = Self.modeFor(app.effectiveAppearance)
      DispatchQueue.main.async { self.apply(mode) }
    }
  }

  public func detach() {
    kvoToken?.invalidate()
    kvoToken = nil
  }

  public func apply(_ mode: Palette.Mode) {
    guard palette.mode != mode else { return }
    let next = Palette.forMode(mode)
    palette = next
    for cb in subscribers.values { cb(next) }
  }

  public func subscribe(_ cb: @escaping (Palette) -> Void) -> UUID {
    let id = UUID()
    subscribers[id] = cb
    return id
  }

  public func unsubscribe(_ id: UUID) {
    subscribers.removeValue(forKey: id)
  }

  public func effectiveAnimationDuration() -> Double {
    reduceMotion() ? 0 : Self.crossfadeDuration
  }

  public static func modeFor(_ appearance: NSAppearance) -> Palette.Mode {
    let match = appearance.bestMatch(from: [.darkAqua, .vibrantDark, .aqua, .vibrantLight])
    switch match {
    case .darkAqua, .vibrantDark: return .dark
    default: return .light
    }
  }
}
