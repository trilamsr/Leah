import SwiftUI

public struct WaveformView: View {
    public let levels: [Float]
    public init(levels: [Float]) { self.levels = levels }

    public var body: some View {
        Canvas { ctx, size in
            guard !levels.isEmpty else { return }
            let bar = size.width / CGFloat(levels.count)
            for (i, lvl) in levels.enumerated() {
                let clamped = CGFloat(max(0, min(1, lvl)))
                let h = clamped * size.height
                let rect = CGRect(
                    x: CGFloat(i) * bar,
                    y: (size.height - h) / 2,
                    width: bar * 0.8,
                    height: h
                )
                ctx.fill(Path(rect), with: .color(.accentColor))
            }
        }
    }
}
