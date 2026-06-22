import Foundation
import LeahIPC

// IPCKeyVerifier sends a verify-key frame to the daemon and awaits the result.
// Daemon-offline degrades gracefully: key is already in Keychain, first request
// will use it. Returns nil on success or degraded-offline; error string on rejection.
public enum IPCKeyVerifier {
  public static let live: (String) async -> String? = { key in
    let client = IPCClient()
    do {
      try await client.connect()
    } catch {
      // Daemon offline — degrade: key saved, will verify on first request.
      return nil
    }
    let result = await verifyWithClient(client, key: key)
    await client.close()
    return result
  }

  private static func verifyWithClient(_ client: IPCClient, key: String) async -> String? {
    let payloadData = (try? JSONEncoder().encode(["key": key])) ?? Data()
    let frame = Frame(
      kind: "verify-key",
      turnId: UUID().uuidString,
      seq: 0,
      payload: RawJSON(payloadData)
    )
    do {
      try await client.send(frame)
      let response = try await client.receive()
      guard response.kind == "verify-key.result" else {
        return "Unexpected response from daemon"
      }
      struct VerifyResult: Decodable { let ok: Bool }
      if let r = try? JSONDecoder().decode(VerifyResult.self, from: response.payload.data), r.ok {
        return nil
      }
      return "Key rejected by Anthropic — check the key and try again."
    } catch {
      return nil // connection dropped mid-flight — degrade
    }
  }
}
