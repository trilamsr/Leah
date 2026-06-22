import SwiftUI

public enum WidgetSize: String, Codable {
    case small, medium, large
}

// Consumed by Focus panel (Task 11) to enumerate and render widgets uniformly.
public protocol LeahWidget: View {
    var size: WidgetSize { get }
}
