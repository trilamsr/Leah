import Foundation
import LeahIPC

// Wraps a verify-key frame round-trip with a hard deadline so the wizard never
// hangs on a deaf daemon. Outcomes — success: key accepted; rejected: daemon
// answered with ok=false; offline: connect/timeout/drop, treated as degraded
// (key already in Keychain, real call validates on first use).
//
// Cancellation: the timeout race uses withTaskGroup.cancelAll() to abandon a
// losing ping. IPCClient.receive() wraps a blocking Darwin.read on a private
// actor, so cancelAll() cannot interrupt an in-flight read — the caller sees
// the timeout immediately, but the orphan ping task lingers until the OS
// closes the socket. A late daemon reply resolves against the already-drained
// TaskGroup and is silently dropped. Acceptable for a first-launch one-shot.
public enum IPCKeyVerifyOutcome: Sendable, Equatable {
  case success
  case rejected(reason: String)
  case offline
}

public enum IPCKeyVerifier {
  public static let defaultTimeout: TimeInterval = 3.0

  public static let live: (String) async -> IPCKeyVerifyOutcome = { key in
    await verify(key: key, timeout: defaultTimeout)
  }

  public static func verify(
    key: String,
    timeout: TimeInterval,
    socketPath: String = IPCClient.defaultSocketPath
  ) async -> IPCKeyVerifyOutcome {
    await withTaskGroup(of: IPCKeyVerifyOutcome.self) { group in
      group.addTask { await pingDaemon(key: key, socketPath: socketPath) }
      group.addTask {
        try? await Task.sleep(nanoseconds: UInt64(max(0, timeout) * 1_000_000_000))
        return .offline
      }
      let first = await group.next() ?? .offline
      group.cancelAll()
      return first
    }
  }

  private static func pingDaemon(key: String, socketPath: String) async -> IPCKeyVerifyOutcome {
    let client = IPCClient(socketPath: socketPath)
    do { try await client.connect() } catch {
      return .offline
    }
    let outcome = await sendAndReceive(client: client, key: key)
    await client.close()
    return outcome
  }

  private static func sendAndReceive(client: IPCClient, key: String) async -> IPCKeyVerifyOutcome {
    let payload = (try? JSONEncoder().encode(["key": key])) ?? Data()
    let frame = Frame(
      kind: "verify-key",
      turnId: UUID().uuidString,
      seq: 0,
      payload: RawJSON(payload)
    )
    do {
      try await client.send(frame)
      let response = try await client.receive()
      guard response.kind == "verify-key.result" else { return .offline }
      struct R: Decodable { let ok: Bool; let reason: String? }
      if let r = try? JSONDecoder().decode(R.self, from: response.payload.data) {
        return r.ok ? .success : .rejected(reason: r.reason ?? "Key rejected.")
      }
      return .offline
    } catch {
      return .offline
    }
  }
}
