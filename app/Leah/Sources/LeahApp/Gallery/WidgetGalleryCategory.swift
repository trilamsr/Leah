import Foundation

public enum WidgetGalleryCategory: String, CaseIterable {
  case finance, travel, time, productivity, web, code

  public var displayName: String {
    switch self {
    case .finance:      return "Finance"
    case .travel:       return "Travel"
    case .time:         return "Time"
    case .productivity: return "Productivity"
    case .web:          return "Web"
    case .code:         return "Code"
    }
  }
}
