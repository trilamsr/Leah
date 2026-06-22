import XCTest
import Darwin
@testable import LeahUI
@testable import LeahIPC

// Exercises IPCKeyVerifier's wire-decode path end-to-end through a real
// AF_UNIX socket — covers the verify-key.result frame the daemon-mocked
// APIKeyStep tests skip. The fixture below is a one-shot echo server:
// accept one connection, read one length-prefixed verify-key frame,
// write one canned response frame, close.
final class IPCKeyVerifierDecodeTests: XCTestCase {

  func testDecodeOkTrueReturnsSuccess() async throws {
    let server = try LocalUnixEchoServer(response: Self.frame(ok: true))
    defer { server.shutdown() }
    let outcome = await IPCKeyVerifier.verify(
      key: "sk-ant-anything",
      timeout: 2.0,
      socketPath: server.path
    )
    XCTAssertEqual(outcome, .success)
  }

  func testDecodeOkFalseReturnsRejectedWithReason() async throws {
    let server = try LocalUnixEchoServer(response: Self.frame(ok: false, reason: "bad key"))
    defer { server.shutdown() }
    let outcome = await IPCKeyVerifier.verify(
      key: "sk-ant-bad",
      timeout: 2.0,
      socketPath: server.path
    )
    XCTAssertEqual(outcome, .rejected(reason: "bad key"))
  }

  func testDecodeWrongKindReturnsOffline() async throws {
    let server = try LocalUnixEchoServer(response: Self.frame(kind: "other"))
    defer { server.shutdown() }
    let outcome = await IPCKeyVerifier.verify(
      key: "sk-ant-anything",
      timeout: 2.0,
      socketPath: server.path
    )
    XCTAssertEqual(outcome, .offline)
  }

  // Encodes a verify-key.result frame the same way the production daemon does:
  // 4-byte big-endian length prefix + JSON body. `kind` defaults match the happy
  // path; pass kind:"other" to exercise the wrong-kind branch.
  private static func frame(kind: String = "verify-key.result", ok: Bool = true, reason: String? = nil) -> Data {
    let payload: Data
    if kind == "verify-key.result" {
      if let reason {
        payload = try! JSONSerialization.data(withJSONObject: ["ok": ok, "reason": reason])
      } else {
        payload = try! JSONSerialization.data(withJSONObject: ["ok": ok])
      }
    } else {
      payload = Data("{}".utf8)
    }
    let f = Frame(kind: kind, turnId: "t", seq: 0, payload: RawJSON(payload))
    return try! FrameCodec.encode(f)
  }
}

// Minimal AF_UNIX echo server: binds a temp socket, accepts ONE connection,
// drains the inbound length-prefixed verify-key frame, writes the canned
// response, closes. Real socket round-trip — no mocking of IPCClient.
final class LocalUnixEchoServer {
  let path: String
  private let listenFD: Int32
  private let serverTask: Task<Void, Never>

  init(response: Data) throws {
    let tmp = "/tmp/lh-echo-\(UUID().uuidString.prefix(8)).sock"
    unlink(tmp)
    let fd = socket(AF_UNIX, SOCK_STREAM, 0)
    guard fd >= 0 else { throw POSIXError(.EIO) }
    var addr = sockaddr_un()
    addr.sun_family = sa_family_t(AF_UNIX)
    let pathBytes = Array(tmp.utf8) + [0]
    guard pathBytes.count <= MemoryLayout.size(ofValue: addr.sun_path) else {
      Darwin.close(fd)
      throw POSIXError(.ENAMETOOLONG)
    }
    withUnsafeMutableBytes(of: &addr.sun_path) { dst in
      pathBytes.withUnsafeBytes { src in dst.copyMemory(from: src) }
    }
    let bindRC = withUnsafePointer(to: &addr) { ptr -> Int32 in
      ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) {
        Darwin.bind(fd, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
      }
    }
    guard bindRC == 0 else { Darwin.close(fd); throw POSIXError(.EADDRINUSE) }
    guard Darwin.listen(fd, 1) == 0 else { Darwin.close(fd); throw POSIXError(.EIO) }
    self.path = tmp
    self.listenFD = fd
    self.serverTask = Task.detached {
      let client = Darwin.accept(fd, nil, nil)
      guard client >= 0 else { return }
      // Drain inbound: 4-byte BE length + body.
      var hdr = [UInt8](repeating: 0, count: 4)
      _ = hdr.withUnsafeMutableBufferPointer { Darwin.read(client, $0.baseAddress, 4) }
      let n = UInt32(hdr[0]) << 24 | UInt32(hdr[1]) << 16 | UInt32(hdr[2]) << 8 | UInt32(hdr[3])
      if n > 0 {
        var body = [UInt8](repeating: 0, count: Int(n))
        var got = 0
        while got < Int(n) {
          let r = body.withUnsafeMutableBufferPointer { Darwin.read(client, $0.baseAddress?.advanced(by: got), Int(n) - got) }
          if r <= 0 { break }
          got += r
        }
      }
      // Write canned response.
      response.withUnsafeBytes { raw in
        guard let base = raw.baseAddress else { return }
        var sent = 0
        while sent < response.count {
          let w = Darwin.write(client, base.advanced(by: sent), response.count - sent)
          if w <= 0 { break }
          sent += w
        }
      }
      Darwin.close(client)
    }
  }

  func shutdown() {
    serverTask.cancel()
    Darwin.close(listenFD)
    unlink(path)
  }
}
