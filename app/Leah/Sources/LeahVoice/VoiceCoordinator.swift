import Foundation
import LeahIPC

public struct TranscriptUpdate: Sendable, Equatable {
    public let text: String
    public let isFinal: Bool
    public init(text: String, isFinal: Bool) {
        self.text = text
        self.isFinal = isFinal
    }
}

// VoiceIPC is the dep-inversion seam against LeahIPC.IPCClient — keeps the
// coordinator testable without spinning up an AF_UNIX socket.
public protocol VoiceIPC: Sendable {
    func send(kind: String, turnId: String, payload: [String: Any]) async throws
}

public protocol VoiceCoordinator: AnyObject, Sendable {
    func startSession(voiceOnly: Bool) async throws
    func endSession() async
    func barge() async
    var transcriptStream: AsyncStream<TranscriptUpdate> { get }
    var levelStream: AsyncStream<Float> { get }
}

// Lazy AsyncStreams are initialized once and never reassigned; AsyncStream.Continuation
// yield/finish are thread-safe, so concurrent ingest() callers do not race.
public final class DefaultVoiceCoordinator: VoiceCoordinator, @unchecked Sendable {
    private let ipc: VoiceIPC
    private let turnId: String
    private var transcriptCont: AsyncStream<TranscriptUpdate>.Continuation?
    private var levelCont: AsyncStream<Float>.Continuation?

    public lazy var transcriptStream: AsyncStream<TranscriptUpdate> = AsyncStream { cont in
        self.transcriptCont = cont
    }
    public lazy var levelStream: AsyncStream<Float> = AsyncStream { cont in
        self.levelCont = cont
    }

    public init(ipc: VoiceIPC, turnId: String = UUID().uuidString) {
        self.ipc = ipc
        self.turnId = turnId
    }

    public func startSession(voiceOnly: Bool) async throws {
        try await ipc.send(kind: "voice.start", turnId: turnId, payload: ["voiceOnly": voiceOnly, "source": "hotkey"])
    }

    public func barge() async {
        try? await ipc.send(kind: "voice.barge", turnId: turnId, payload: [:])
    }

    public func endSession() async {
        try? await ipc.send(kind: "voice.end", turnId: turnId, payload: [:])
        transcriptCont?.finish()
        levelCont?.finish()
    }

    public func ingest(transcript: TranscriptUpdate) {
        transcriptCont?.yield(transcript)
    }

    public func ingest(level: Float) {
        levelCont?.yield(level)
    }
}
