import SwiftUI

// MinimalGuard strips ornamental layers (grain overlay + italic) when the
// AppearancePane toggle is on; default branch preserves today's render.
// Perf-review item #2: a multiply blend on a translucent black tile is
// algebraically equivalent to sourceOver and forces an offscreen pass per
// frame. Drop the blend; keep the visual.
public struct MinimalGuard: ViewModifier {
    public static let overlayOpacity: Double = 0.04
    public static let usesBlendCompositing: Bool = false

    public init() {}
    public func body(content: Content) -> some View {
        if Palette.minimalMode {
            content.environment(\.font, .system(.body))
        } else {
            content.overlay(
                Color.black.opacity(Self.overlayOpacity)
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
