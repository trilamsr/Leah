import Foundation
#if canImport(AppKit)
import AppKit
#endif

public struct HotkeyBinding: Sendable, Equatable {
    public struct Modifiers: OptionSet, Sendable {
        public let rawValue: Int
        public init(rawValue: Int) { self.rawValue = rawValue }
        public static let option = Modifiers(rawValue: 1 << 0)
        public static let shift  = Modifiers(rawValue: 1 << 1)
        public static let cmd    = Modifiers(rawValue: 1 << 2)
        public static let ctrl   = Modifiers(rawValue: 1 << 3)
    }
    public let keyCode: Int
    public let modifiers: Modifiers
}

public final class SnapTool: @unchecked Sendable {
    public static let hotkey = HotkeyBinding(keyCode: 49, modifiers: [.option, .shift])

    private let ipc: VisionIPC
    private let turnId: String
    private var streamCont: AsyncStream<StreamFrame>.Continuation?

    public lazy var streamFrames: AsyncStream<StreamFrame> = AsyncStream { cont in
        self.streamCont = cont
    }

    public init(ipc: VisionIPC, turnId: String = UUID().uuidString) {
        self.ipc = ipc
        self.turnId = turnId
    }

    public func snap(prompt: String, mode: VisionMode, rect: SnapRect?) async throws {
        var payload: [String: Any] = ["prompt": prompt, "mode": mode.rawValue]
        if let rect { payload["rect"] = rect.asPayload }
        try await ipc.send(kind: "vision.snap", turnId: turnId, payload: payload)
    }

    public func startStream(source: String, fps: Int) async throws {
        try await ipc.send(
            kind: "vision.stream.start",
            turnId: turnId,
            payload: ["source": source, "fps": fps]
        )
    }

    public func ingest(streamFrame: StreamFrame) {
        streamCont?.yield(streamFrame)
    }

    public func finish() {
        streamCont?.finish()
    }
}
