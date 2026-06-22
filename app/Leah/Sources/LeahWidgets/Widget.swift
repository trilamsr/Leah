import SwiftUI

public enum WidgetSize: String, Codable {
    case small, medium, large
}

public protocol LeahWidget: View {
    var size: WidgetSize { get }
}
