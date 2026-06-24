import SwiftUI

public enum WidgetGalleryAffordance: Equatable {
  case plusButton, hotkey, slashCommand
}

@MainActor
public final class WidgetGalleryModel: ObservableObject {
  @Published public var isVisible: Bool = false
  @Published public var searchQuery: String = ""
  @Published public var selectedCategory: WidgetGalleryCategory = .finance
  @Published public private(set) var lastAffordance: WidgetGalleryAffordance? = nil

  public var onSpawn: ((WidgetGalleryItem) -> Void)? = nil

  public init() {}

  public func openFromPlusButton() {
    lastAffordance = .plusButton
    isVisible = true
  }

  public func openFromHotkey() {
    lastAffordance = .hotkey
    isVisible = true
  }

  /// Treats a literal `/widgets` token as toggle: opens when closed, dismisses
  /// when already open (spec §10.4 dismiss row).
  /// Returns true if the input was a /widgets command and was consumed.
  @discardableResult
  public func consumeSlashIfPresent(_ text: String) -> Bool {
    let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
    guard trimmed == "/widgets" else { return false }
    if isVisible {
      dismiss()
    } else {
      lastAffordance = .slashCommand
      isVisible = true
    }
    return true
  }

  public func handleEscape() {
    dismiss()
  }

  public func dismiss() {
    isVisible = false
  }

  public func spawn(_ item: WidgetGalleryItem) {
    onSpawn?(item)
    dismiss()
  }

  public var filteredItems: [WidgetGalleryItem] {
    let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    if q.isEmpty { return WidgetGalleryCatalog.items }
    return WidgetGalleryCatalog.items.filter { item in
      fuzzyMatch(needle: q, haystack: item.name.lowercased())
        || fuzzyMatch(needle: q, haystack: item.category.displayName.lowercased())
        || fuzzyMatch(needle: q, haystack: item.sampleText.lowercased())
    }
  }

  // Subsequence fuzzy match: every char in needle appears in haystack in order.
  // Keeps "AAPL" finding "AAPL price ticker sparkline" and "mrkt" finding "Market".
  private func fuzzyMatch(needle: String, haystack: String) -> Bool {
    if haystack.contains(needle) { return true }
    var i = needle.startIndex
    for ch in haystack {
      if i == needle.endIndex { return true }
      if ch == needle[i] { i = needle.index(after: i) }
    }
    return i == needle.endIndex
  }
}

public struct WidgetGalleryView: View {
  @ObservedObject var model: WidgetGalleryModel
  let registry: WidgetTileRegistry

  public init(model: WidgetGalleryModel, registry: WidgetTileRegistry) {
    self.model = model
    self.registry = registry
  }

  public var body: some View {
    HStack(spacing: 0) {
      categoryRail
        .frame(width: 160)
      Divider().background(LeahPalette.hairline)
      VStack(alignment: .leading, spacing: 12) {
        TextField("Find a widget…", text: $model.searchQuery)
          .textFieldStyle(.plain)
          .font(.body)
          .foregroundColor(.leahIvory)
          .padding(.horizontal, 12)
          .padding(.vertical, 8)
          .background(LeahPalette.obsidian2)
          .overlay(
            RoundedRectangle(cornerRadius: 4).stroke(LeahPalette.hairline, lineWidth: 1)
          )
        previewGrid
      }
      .padding(16)
    }
    .background(Color(red: 8/255, green: 9/255, blue: 12/255))
    .onExitCommand { model.handleEscape() }
    .transition(.opacity.animation(.easeOut(duration: 0.16)))
  }

  private var categoryRail: some View {
    VStack(alignment: .leading, spacing: 10) {
      ForEach(WidgetGalleryCategory.allCases, id: \.self) { cat in
        Text(cat.displayName.uppercased())
          .font(.caption.weight(.medium))
          .tracking(0.04 * 11)
          .foregroundColor(cat == model.selectedCategory ? .leahGold : LeahPalette.textMuted)
          .onTapGesture { model.selectedCategory = cat }
      }
      Spacer()
    }
    .padding(16)
  }

  private var previewGrid: some View {
    let items = model.filteredItems.filter {
      model.searchQuery.isEmpty ? $0.category == model.selectedCategory : true
    }
    return ScrollView {
      LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 12) {
        ForEach(items) { item in
          previewCell(item)
        }
      }
    }
  }

  private func previewCell(_ item: WidgetGalleryItem) -> some View {
    VStack(alignment: .leading, spacing: 6) {
      Text(item.name.uppercased())
        .font(.caption2.weight(.medium))
        .tracking(0.04 * 10)
        .foregroundColor(LeahPalette.textMuted)
      if let live = registry.view(for: item.sampleEnvelope()) {
        live
      } else {
        Text(item.sampleText)
          .font(.caption)
          .foregroundColor(.leahIvory)
      }
      Button("Spawn") { model.spawn(item) }
        .buttonStyle(.plain)
        .font(.caption.weight(.medium))
        .foregroundColor(.leahGold)
    }
    .padding(10)
    .frame(maxWidth: .infinity, minHeight: 96, alignment: .topLeading)
    .overlay(
      RoundedRectangle(cornerRadius: 8).stroke(LeahPalette.hairline, lineWidth: 1)
    )
  }
}
