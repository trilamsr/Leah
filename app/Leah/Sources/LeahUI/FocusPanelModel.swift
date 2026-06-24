import Foundation
import Combine
import LeahIPC

// Transport seam — lets tests drive send/receive without a real socket.
// IPCClient conforms via the extension below, so production wiring stays the
// same FocusPanelView(client:) call.
public protocol FocusPanelTransport: AnyObject, Sendable {
  func send(_ frame: Frame) async throws
  func receive() async throws -> Frame
}

extension IPCClient: FocusPanelTransport {}

// .connecting before any successful frame; .connected after the first send or
// receive succeeds; .error when transport throws (model schedules a 2s
// reconnect probe and stays in .error until a transport call succeeds).
public enum FocusPanelConnection: Equatable {
  case connecting
  case connected
  case error(reason: String)
}

@MainActor
public final class FocusPanelModel: ObservableObject {
  @Published public var input: String = ""
  // Each user turn appends one line; prose.delta deltas mutate the last line.
  // Storing as a list (not one String) lets ScrollViewReader anchor on a
  // stable id for deterministic auto-scroll.
  @Published public private(set) var lines: [Line] = []
  @Published public private(set) var connection: FocusPanelConnection = .connecting
  // Auto-scroll latch: true → new content pins to bottom. Manual drag-up sets
  // false; bottom-tap restores true.
  @Published public var autoScroll: Bool = true
  // Bounded ring — Canvas redraws on every yield, so 64 bars keep paint cheap.
  @Published public private(set) var levels: [Float] = []

  public struct Line: Identifiable, Equatable {
    public let id: UUID
    public var text: String
    public init(id: UUID = UUID(), text: String) {
      self.id = id
      self.text = text
    }
  }

  public static let levelBufferSize = 64
  public static let reconnectDelayNanos: UInt64 = 2_000_000_000

  private let transport: FocusPanelTransport?
  // Injected sleep so tests can advance the reconnect loop without wall-clock
  // waits. Production passes Task.sleep.
  private let sleep: @Sendable (UInt64) async throws -> Void

  public init(
    transport: FocusPanelTransport?,
    sleep: @escaping @Sendable (UInt64) async throws -> Void = { try await Task.sleep(nanoseconds: $0) }
  ) {
    self.transport = transport
    self.sleep = sleep
  }

  public func markConnected() { connection = .connected }
  public func markError(_ reason: String) { connection = .error(reason: reason) }

  // User-driven retry. Sends a no-op probe; flips to .connected on success,
  // back to .error on throw. Returns once the probe resolves.
  public func retry() async {
    guard let transport else { return }
    connection = .connecting
    do {
      let probe = Frame(kind: "ping", turnId: UUID().uuidString, seq: 0, payload: RawJSON(Data("{}".utf8)))
      try await transport.send(probe)
      connection = .connected
    } catch {
      connection = .error(reason: "\(error)")
    }
  }

  // While in .error, sleep then probe. Exits when connection flips or task is
  // cancelled — caller owns the Task so dismiss() can cancel without leaking.
  public func reconnectLoop() async {
    while case .error = connection {
      do { try await sleep(Self.reconnectDelayNanos) } catch { return }
      await retry()
    }
  }

  public func appendLevel(_ lvl: Float) {
    levels.append(lvl)
    if levels.count > Self.levelBufferSize {
      levels.removeFirst(levels.count - Self.levelBufferSize)
    }
  }

  public func clearLevels() { levels.removeAll() }

  // Drains an AsyncStream<Float> into the ring buffer. Caller spawns this in
  // a Task tied to TTS playback; cancelling the task stops ingestion.
  public func bind(levelStream: AsyncStream<Float>) async {
    for await lvl in levelStream {
      appendLevel(lvl)
    }
  }

  // Starts a new turn: appends an empty response line and returns its id so
  // the view can anchor ScrollViewReader on it.
  @discardableResult
  public func beginTurn() -> UUID {
    let line = Line(text: "")
    lines.append(line)
    return line.id
  }

  // Streaming delta append. No-op if id is unknown (turn cleared mid-stream).
  public func append(delta: String, to id: UUID) {
    guard let idx = lines.firstIndex(where: { $0.id == id }) else { return }
    lines[idx].text.append(delta)
  }

  // Sends ask + drains prose.delta until turn.end. Flips to .connected on
  // first frame; on throw flips to .error and returns.
  public func send(text: String) async {
    guard let transport, !text.isEmpty else { return }
    input = ""
    let turnID = UUID().uuidString
    let lineID = beginTurn()
    do {
      let askPayload = try JSONEncoder().encode(["text": text])
      let frame = Frame(kind: "ask", turnId: turnID, seq: 0, payload: RawJSON(askPayload))
      try await transport.send(frame)
      while true {
        let f = try await transport.receive()
        if connection != .connected { connection = .connected }
        if f.kind == "prose.delta" {
          if let obj = try? JSONDecoder().decode([String: String].self, from: f.payload.data),
             let delta = obj["text"] {
            append(delta: delta, to: lineID)
          }
        } else if f.kind == "turn.end" {
          return
        }
      }
    } catch {
      connection = .error(reason: "\(error)")
    }
  }
}
