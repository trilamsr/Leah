import XCTest
import LeahIPC
@testable import LeahUI

// Drives the model without a real AF_UNIX socket. Each test plans the
// `receive` script up front; throws on demand to exercise error paths.
final class MockTransport: FocusPanelTransport, @unchecked Sendable {
  enum Step {
    case frame(Frame)
    case error(Error)
  }
  var script: [Step] = []
  var sendError: Error?
  var sendCount: Int = 0
  var sent: [Frame] = []

  func send(_ frame: Frame) async throws {
    sendCount += 1
    sent.append(frame)
    if let e = sendError { throw e }
  }

  func receive() async throws -> Frame {
    guard !script.isEmpty else { throw NSError(domain: "MockTransport", code: 1) }
    switch script.removeFirst() {
    case .frame(let f): return f
    case .error(let e): throw e
    }
  }
}

private func proseDelta(_ text: String, turnID: String = "t1") throws -> Frame {
  let payload = try JSONEncoder().encode(["text": text])
  return Frame(kind: "prose.delta", turnId: turnID, seq: 0, payload: RawJSON(payload))
}

private func turnEnd(_ turnID: String = "t1") -> Frame {
  Frame(kind: "turn.end", turnId: turnID, seq: 0, payload: RawJSON(Data("{}".utf8)))
}

final class FocusPanelModelTests: XCTestCase {

  // Waveform binding: model drains the AsyncStream<Float> into its ring
  // buffer; view renders only when levels is non-empty, so empty→non-empty
  // is the load-bearing animation gate during TTS playback.
  @MainActor
  func testPanelWaveformAnimatesDuringTTS() async {
    let model = FocusPanelModel(transport: nil)
    let (stream, cont) = AsyncStream<Float>.makeStream()
    XCTAssertTrue(model.levels.isEmpty, "starts with no levels (waveform hidden)")
    let task = Task { await model.bind(levelStream: stream) }
    cont.yield(0.1); cont.yield(0.5); cont.yield(0.9)
    cont.finish()
    await task.value
    XCTAssertEqual(model.levels, [0.1, 0.5, 0.9], "levels drained in order")
  }

  // Ring buffer cap — 64 samples max so Canvas redraw stays cheap during
  // long TTS streams.
  @MainActor
  func testPanelWaveformRingBufferBounded() {
    let model = FocusPanelModel(transport: nil)
    for i in 0..<200 {
      model.appendLevel(Float(i) / 200.0)
    }
    XCTAssertEqual(model.levels.count, FocusPanelModel.levelBufferSize)
    // Newest sample survives — old drops, recent paints.
    XCTAssertEqual(model.levels.last ?? -1, Float(199.0 / 200.0), accuracy: 0.0001)
  }

  // Auto-scroll behavior: new lines appended while autoScroll is on keep
  // the latch set. The view binds the latch to ScrollViewReader.scrollTo;
  // model-level invariant is simply that autoScroll stays true.
  @MainActor
  func testPanelAutoScrollOnAppend() {
    let model = FocusPanelModel(transport: nil)
    XCTAssertTrue(model.autoScroll, "default is auto-scroll on")
    let id = model.beginTurn()
    model.append(delta: "hello ", to: id)
    model.append(delta: "world", to: id)
    XCTAssertTrue(model.autoScroll, "appending must not turn off auto-scroll")
    XCTAssertEqual(model.lines.last?.text, "hello world")
  }

  // Manual scroll-up flips the latch off — the view's DragGesture sets
  // autoScroll = false on a downward content drag.
  @MainActor
  func testPanelStopsAutoScrollOnUserScroll() {
    let model = FocusPanelModel(transport: nil)
    XCTAssertTrue(model.autoScroll)
    model.autoScroll = false
    let id = model.beginTurn()
    model.append(delta: "more text", to: id)
    XCTAssertFalse(model.autoScroll, "user-pinned state must survive new deltas")
  }

  // Bottom-tap restores the latch. The view binds the "Jump to latest"
  // button to autoScroll = true; model invariant is just the toggle.
  @MainActor
  func testPanelResumesOnBottomTap() {
    let model = FocusPanelModel(transport: nil)
    model.autoScroll = false
    model.autoScroll = true
    XCTAssertTrue(model.autoScroll)
  }

  // Empty state: no transport bound = .connecting forever until something
  // marks it connected. With no lines, the view renders the empty pane.
  @MainActor
  func testPanelEmptyStateOnNoDaemon() {
    let model = FocusPanelModel(transport: nil)
    XCTAssertEqual(model.connection, .connecting)
    XCTAssertTrue(model.lines.isEmpty)
  }

  // Error state: a throwing send flips connection to .error; the reconnect
  // loop is gated by the connection state so a no-op sleep loop terminates
  // once retry() succeeds.
  @MainActor
  func testPanelErrorStateOnSocketBroken() async {
    let transport = MockTransport()
    transport.sendError = NSError(domain: "EPIPE", code: 32)
    let model = FocusPanelModel(transport: transport, sleep: { _ in })
    await model.send(text: "hi")
    if case .error = model.connection {
      // expected
    } else {
      XCTFail("send error must flip connection to .error, got \(model.connection)")
    }
  }

  // Reconnect loop: stays in .error until a probe (retry()) succeeds, then
  // exits. Using sleep injection so the test does not wait 2s.
  @MainActor
  func testReconnectLoopExitsOnSuccessfulProbe() async {
    let transport = MockTransport()
    transport.sendError = NSError(domain: "EPIPE", code: 32)
    let model = FocusPanelModel(transport: transport, sleep: { _ in })
    model.markError("seeded")
    let task = Task { await model.reconnectLoop() }
    // First retry fails; clear the error so the second probe lands.
    try? await Task.sleep(nanoseconds: 50_000_000)
    transport.sendError = nil
    await task.value
    XCTAssertEqual(model.connection, .connected)
  }

  // Streaming happy path: send() emits ask, drains prose.delta, returns on
  // turn.end. After turn.end, connection is .connected and lines hold the
  // full transcript.
  @MainActor
  func testSendStreamsDeltasUntilTurnEnd() async throws {
    let transport = MockTransport()
    transport.script = [
      .frame(try proseDelta("hello ")),
      .frame(try proseDelta("world")),
      .frame(turnEnd()),
    ]
    let model = FocusPanelModel(transport: transport, sleep: { _ in })
    await model.send(text: "prompt")
    XCTAssertEqual(model.connection, .connected)
    XCTAssertEqual(model.lines.last?.text, "hello world")
    XCTAssertEqual(transport.sent.first?.kind, "ask")
  }

  // Retry button: user-driven retry sends a probe and flips state on
  // success. No transport = no-op (preview/gallery harnesses).
  @MainActor
  func testRetrySucceedsWithLiveTransport() async {
    let transport = MockTransport()
    let model = FocusPanelModel(transport: transport, sleep: { _ in })
    model.markError("seeded")
    await model.retry()
    XCTAssertEqual(model.connection, .connected)
    XCTAssertEqual(transport.sendCount, 1, "retry must send a probe")
  }
}
