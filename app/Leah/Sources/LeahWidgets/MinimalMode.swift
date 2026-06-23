import SwiftUI

// MinimalGuard strips ornamental layers (grain overlay + italic) when the
// AppearancePane toggle is on; default branch preserves today's render.
public struct MinimalGuard: ViewModifier {
    public init() {}
    public func body(content: Content) -> some View {
        if Palette.minimalMode {
            content.environment(\.font, .system(.body))
        } else {
            content.overlay(
                Color.black.opacity(0.04)
                    .blendMode(.multiply)
                    .allowsHitTesting(false)
            )
        }
    }
}

extension View {
    public func respectsMinimalMode() -> some View {
        modifier(MinimalGuard())
    }
}
