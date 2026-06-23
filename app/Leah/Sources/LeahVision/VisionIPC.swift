import Foundation

public protocol VisionIPC: Sendable {
    func send(kind: String, turnId: String, payload: [String: Any]) async throws
}

public enum VisionMode: Int, Sendable {
    case local = 0
    case sonnet = 1
    case auto = 2
}

public struct SnapRect: Sendable, Equatable {
    public let x: Int
    public let y: Int
    public let width: Int
    public let height: Int
    public init(x: Int, y: Int, width: Int, height: Int) {
        self.x = x; self.y = y; self.width = width; self.height = height
    }
    public var asPayload: [String: Int] {
        ["x": x, "y": y, "width": width, "height": height]
    }
}

public struct StreamFrame: Sendable, Equatable {
    public let thumbBase64: String
    public let summary: String?
    public init(thumbBase64: String, summary: String? = nil) {
        self.thumbBase64 = thumbBase64
        self.summary = summary
    }
}
