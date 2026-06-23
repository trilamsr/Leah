import Foundation
import SwiftUI

public struct SelectionOverlay: View {
    @State private var anchor: CGPoint?
    @State private var current: CGPoint?
    public let onCommit: (SnapRect) -> Void
    public let onCancel: () -> Void

    public init(onCommit: @escaping (SnapRect) -> Void, onCancel: @escaping () -> Void) {
        self.onCommit = onCommit
        self.onCancel = onCancel
    }

    public var body: some View {
        GeometryReader { _ in
            ZStack {
                Color.black.opacity(0.25)
                    .ignoresSafeArea()
                if let rect = currentRect {
                    Rectangle()
                        .strokeBorder(Color.accentColor, lineWidth: 2)
                        .background(Color.accentColor.opacity(0.1))
                        .frame(width: rect.width, height: rect.height)
                        .position(x: rect.midX, y: rect.midY)
                }
            }
            .contentShape(Rectangle())
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { value in
                        if anchor == nil { anchor = value.startLocation }
                        current = value.location
                    }
                    .onEnded { _ in
                        guard let rect = currentRect, rect.width > 4, rect.height > 4 else {
                            onCancel(); return
                        }
                        onCommit(SnapRect(
                            x: Int(rect.minX),
                            y: Int(rect.minY),
                            width: Int(rect.width),
                            height: Int(rect.height)
                        ))
                    }
            )
            .onExitCommand { onCancel() }
        }
    }

    private var currentRect: CGRect? {
        guard let a = anchor, let c = current else { return nil }
        return CGRect(
            x: min(a.x, c.x),
            y: min(a.y, c.y),
            width: abs(c.x - a.x),
            height: abs(c.y - a.y)
        )
    }
}
