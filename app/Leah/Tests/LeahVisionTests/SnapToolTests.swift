import XCTest
@testable import LeahVision

final class SnapToolTests: XCTestCase {
    func testSnapFullScreenEmitsVisionSnap() async throws {
        let ipc = MockVisionIPC()
        let tool = SnapTool(ipc: ipc, turnId: "t1")
        try await tool.snap(prompt: "what is this?", mode: .auto, rect: nil)
        let frames = await ipc.frames()
        XCTAssertEqual(frames.map { $0.kind }, ["vision.snap"])
        let payload = frames[0].payload
        XCTAssertEqual(payload["prompt"] as? String, "what is this?")
        XCTAssertEqual(payload["mode"] as? Int, VisionMode.auto.rawValue)
        XCTAssertNil(payload["rect"])
    }

    func testSnapSelectionIncludesRect() async throws {
        let ipc = MockVisionIPC()
        let tool = SnapTool(ipc: ipc, turnId: "t2")
        let rect = SnapRect(x: 10, y: 20, width: 300, height: 200)
        try await tool.snap(prompt: "read", mode: .local, rect: rect)
        let frames = await ipc.frames()
        XCTAssertEqual(frames.count, 1)
        let dict = frames[0].payload["rect"] as? [String: Int]
        XCTAssertEqual(dict, ["x": 10, "y": 20, "width": 300, "height": 200])
        XCTAssertEqual(frames[0].payload["mode"] as? Int, VisionMode.local.rawValue)
    }

    func testHotkeyBindingIsAltShiftSpace() {
        let binding = SnapTool.hotkey
        XCTAssertEqual(binding.keyCode, 49)
        XCTAssertTrue(binding.modifiers.contains(.option))
        XCTAssertTrue(binding.modifiers.contains(.shift))
    }

    func testStreamFrameIngestForwardsToSubscribers() async throws {
        let ipc = MockVisionIPC()
        let tool = SnapTool(ipc: ipc, turnId: "t3")
        let stream = tool.streamFrames
        tool.ingest(streamFrame: StreamFrame(thumbBase64: "abc", summary: "hello"))
        tool.finish()
        var seen: [StreamFrame] = []
        for await frame in stream { seen.append(frame) }
        XCTAssertEqual(seen.count, 1)
        XCTAssertEqual(seen[0].summary, "hello")
    }
}

actor MockVisionIPC: VisionIPC {
    struct Sent { let kind: String; let payload: [String: Any] }
    private var sent: [Sent] = []
    func send(kind: String, turnId: String, payload: [String: Any]) async throws {
        sent.append(Sent(kind: kind, payload: payload))
    }
    func frames() -> [Sent] { sent }
}
