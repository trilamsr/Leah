import XCTest
@testable import LeahVoice

final class VoiceCoordinatorTests: XCTestCase {
    func testStartThenEndSendsBothFrames() async throws {
        let ipc = MockIPC()
        let coord = DefaultVoiceCoordinator(ipc: ipc, turnId: "t1")
        try await coord.startSession(voiceOnly: false)
        await coord.endSession()
        let kinds = await ipc.kinds()
        XCTAssertEqual(kinds, ["voice.start", "voice.end"])
    }

    func testBargeEmitsVoiceBarge() async throws {
        let ipc = MockIPC()
        let coord = DefaultVoiceCoordinator(ipc: ipc, turnId: "t2")
        try await coord.startSession(voiceOnly: true)
        await coord.barge()
        let kinds = await ipc.kinds()
        XCTAssertEqual(kinds, ["voice.start", "voice.barge"])
    }

    func testEndSessionFinishesTranscriptStream() async throws {
        let ipc = MockIPC()
        let coord = DefaultVoiceCoordinator(ipc: ipc, turnId: "t3")
        let stream = coord.transcriptStream
        try await coord.startSession(voiceOnly: false)
        coord.ingest(transcript: TranscriptUpdate(text: "hello", isFinal: false))
        await coord.endSession()
        var seen: [TranscriptUpdate] = []
        for await update in stream {
            seen.append(update)
        }
        XCTAssertEqual(seen, [TranscriptUpdate(text: "hello", isFinal: false)])
    }

    func testWaveformViewAcceptsLevels() {
        let view = WaveformView(levels: [0.1, 0.5, 0.9])
        XCTAssertEqual(view.levels.count, 3)
    }
}

actor MockIPC: VoiceIPC {
    private var sent: [(kind: String, payload: [String: Any])] = []

    func send(kind: String, turnId: String, payload: [String: Any]) async throws {
        sent.append((kind, payload))
    }

    func kinds() -> [String] { sent.map { $0.kind } }
}
