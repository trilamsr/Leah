import SwiftUI
import AppKit

public enum DashboardTileSlot: String, CaseIterable, Hashable, Sendable {
  case memory, agenda, briefs, news, knowledge
}

public final class DashboardTileResolver {
  public typealias Resolve = (DashboardTileSlot) -> AnyView

  private let resolve: Resolve
  public private(set) var callCount: [DashboardTileSlot: Int] = [:]

  public init(_ resolve: @escaping Resolve) { self.resolve = resolve }

  public func view(for slot: DashboardTileSlot) -> AnyView {
    callCount[slot, default: 0] += 1
    return resolve(slot)
  }

  public static var placeholder: DashboardTileResolver {
    DashboardTileResolver { _ in AnyView(Color.clear) }
  }
}

public struct DashboardView: View {
  public static let columnCount: Int = 2
  public static let requiredSlots: [DashboardTileSlot] = [.memory, .agenda, .briefs, .news, .knowledge]

  private let resolver: DashboardTileResolver

  public init(resolver: DashboardTileResolver = .placeholder) {
    self.resolver = resolver
  }

  public var body: some View {
    let columns = Array(
      repeating: GridItem(.flexible(), spacing: 20),
      count: DashboardView.columnCount
    )
    return ScrollView {
      VStack(alignment: .leading, spacing: 24) {
        header
        LazyVGrid(columns: columns, spacing: 20) {
          ForEach(DashboardView.requiredSlots, id: \.self) { slot in
            resolver.view(for: slot)
              .frame(maxWidth: .infinity, minHeight: 180, alignment: .topLeading)
              .background(DashboardPalette.obsidian2)
              .overlay(
                RoundedRectangle(cornerRadius: 12)
                  .stroke(DashboardPalette.hairline, lineWidth: 1)
              )
              .clipShape(RoundedRectangle(cornerRadius: 12))
          }
        }
      }
      .padding(32)
      .frame(maxWidth: .infinity, alignment: .topLeading)
    }
    .background(DashboardPalette.obsidian0)
  }

  @ViewBuilder
  private var header: some View {
    let minimal = AppearancePane.minimalModeEnabled()
    Text("Today")
      .font(minimal
        ? .system(size: 36, weight: .regular)
        : .custom("Tiempos Italic", size: 36, relativeTo: .largeTitle))
      .italic()
      .foregroundColor(DashboardPalette.ivory)
  }
}

enum DashboardPalette {
  static let obsidian0  = Color(red: 0x08/255, green: 0x09/255, blue: 0x0C/255)
  static let obsidian2  = Color(red: 0x16/255, green: 0x19/255, blue: 0x22/255)
  static let ivory      = Color(red: 242/255,   green: 237/255,  blue: 224/255)
  static let hairline   = Color.white.opacity(0.20)
}
