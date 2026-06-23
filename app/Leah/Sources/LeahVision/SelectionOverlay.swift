import Foundation
import SwiftUI

public enum SelectionGeometry {
    public static let minSide: CGFloat = 4

    public static func rect(anchor: CGPoint, current: CGPoint) -> CGRect {
        CGRect(
            x: min(anchor.x, current.x),
            y: min(anchor.y, current.y),
            width: abs(current.x - anchor.x),
            height: abs(current.y - anchor.y)
        )
    }

    public static func snap(from rect: CGRect) -> SnapRect? {
        guard rect.width > minSide, rect.height > minSide else { return nil }
        return SnapRect(
            x: Int(rect.minX),
            y: Int(rect.minY),
            width: Int(rect.width),
            height: Int(rect.height)
        )
    }
}

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
                        guard let rect = currentRect, let snap = SelectionGeometry.snap(from: rect) else {
                            onCancel(); return
                        }
                        onCommit(snap)
                    }
            )
            .onExitCommand { onCancel() }
        }
    }

    private var currentRect: CGRect? {
        guard let a = anchor, let c = current else { return nil }
        return SelectionGeometry.rect(anchor: a, current: c)
    }
}
