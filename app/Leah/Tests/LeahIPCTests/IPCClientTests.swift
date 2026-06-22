import XCTest
@testable import LeahIPC

final class FrameCodecTests: XCTestCase {
    func testRoundTripFrame() throws {
        let payload = #"{"text":"hi"}"#.data(using: .utf8)!
        let f = Frame(kind: "prose.delta", turnId: "t1", seq: 7, payload: RawJSON(payload))
        let encoded = try FrameCodec.encode(f)
        // 4-byte length prefix + JSON body
        XCTAssertGreaterThan(encoded.count, 4)
        let (decoded, consumed) = try FrameCodec.decode(encoded)
        XCTAssertEqual(consumed, encoded.count)
        XCTAssertEqual(decoded.kind, "prose.delta")
        XCTAssertEqual(decoded.turnId, "t1")
        XCTAssertEqual(decoded.seq, 7)
    }

    func testFrameSizeCapRejected() {
        // Large valid-JSON string payload exceeding the 256KB cap.
        let huge = "\"" + String(repeating: "x", count: 300_000) + "\""
        let payload = huge.data(using: .utf8)!
        let f = Frame(kind: "prose.delta", turnId: "t1", seq: 1, payload: RawJSON(payload))
        XCTAssertThrowsError(try FrameCodec.encode(f))
    }

    func testAllFrameKindsRoundTrip() throws {
        let kinds = [
            "prose.delta",
            "turn.end",
            "widget.mount",
            "widget.update",
            "widget.stale",
            "widget.error",
            "widget.unmount",
            "error",
        ]
        for kind in kinds {
            let payload = #"{"kind_echo":"\#(kind)"}"#.data(using: .utf8)!
            let f = Frame(kind: kind, turnId: "t-\(kind)", seq: 42, payload: RawJSON(payload))
            let encoded = try FrameCodec.encode(f)
            let (decoded, consumed) = try FrameCodec.decode(encoded)
            XCTAssertEqual(consumed, encoded.count, "consumed mismatch for kind \(kind)")
            XCTAssertEqual(decoded.kind, kind)
            XCTAssertEqual(decoded.turnId, "t-\(kind)")
            XCTAssertEqual(decoded.seq, 42)
        }
    }

    func testDecodeShortHeaderFails() {
        let data = Data([0x00, 0x00]) // only 2 bytes, need 4
        XCTAssertThrowsError(try FrameCodec.decode(data))
    }

    func testDecodeTruncatedBodyFails() {
        // Header says 100 bytes, but only 10 follow
        var data = Data(count: 4)
        let n = UInt32(100).bigEndian
        withUnsafeBytes(of: n) { ptr in data.replaceSubrange(0..<4, with: ptr) }
        data.append(Data(repeating: 0, count: 10))
        XCTAssertThrowsError(try FrameCodec.decode(data))
    }

    func testDecodeOversizeHeaderFails() {
        // Header says 512KB, over the 256KB cap
        var data = Data(count: 4)
        let n = UInt32(512 * 1024).bigEndian
        withUnsafeBytes(of: n) { ptr in data.replaceSubrange(0..<4, with: ptr) }
        XCTAssertThrowsError(try FrameCodec.decode(data))
    }

    func testTurnIdCodingKeyMapping() throws {
        // Verify the JSON wire format uses "turn_id" not "turnId".
        // Use non-empty payload so the payload key check on the next test isn't
        // confused with omitempty behavior.
        let payload = #"{"x":1}"#.data(using: .utf8)!
        let f = Frame(kind: "turn.end", turnId: "xyz", seq: 0, payload: RawJSON(payload))
        let encoded = try FrameCodec.encode(f)
        let jsonBody = encoded.dropFirst(4)
        let jsonObj = try JSONSerialization.jsonObject(with: jsonBody) as! [String: Any]
        XCTAssertNotNil(jsonObj["turn_id"], "wire key must be turn_id")
        XCTAssertNil(jsonObj["turnId"], "camelCase key must not appear on wire")
        XCTAssertEqual(jsonObj["turn_id"] as? String, "xyz")
    }

    func testEmptyPayloadOmittedOnWire() throws {
        // Mirrors Go's `omitempty`: empty payload must not produce a "payload" key.
        let f = Frame(kind: "turn.end", turnId: "t1", seq: 1, payload: RawJSON(Data()))
        let encoded = try FrameCodec.encode(f)
        let jsonBody = encoded.dropFirst(4)
        let jsonObj = try JSONSerialization.jsonObject(with: jsonBody) as! [String: Any]
        XCTAssertNil(jsonObj["payload"], "empty payload must be omitted on wire (parity with Go omitempty)")
    }

    func testMissingPayloadDecodesAsEmpty() throws {
        // Frames from Go with omitempty payload arrive with no "payload" key.
        let body = #"{"kind":"turn.end","turn_id":"t1","seq":1}"#.data(using: .utf8)!
        var encoded = Data(count: 4)
        let n = UInt32(body.count).bigEndian
        withUnsafeBytes(of: n) { ptr in encoded.replaceSubrange(0..<4, with: ptr) }
        encoded.append(body)
        let (frame, _) = try FrameCodec.decode(encoded)
        XCTAssertEqual(frame.payload.data, Data())
    }

    func testGoWireFormatDecodes() throws {
        // Wire format from Go daemon: payload is a literal JSON object, not base64.
        let json = #"{"kind":"prose.delta","turn_id":"t1","seq":7,"payload":{"text":"hi"}}"#.data(using: .utf8)!
        var encoded = Data(count: 4)
        let n = UInt32(json.count).bigEndian
        withUnsafeBytes(of: n) { ptr in encoded.replaceSubrange(0..<4, with: ptr) }
        encoded.append(json)
        let (f, _) = try FrameCodec.decode(encoded)
        XCTAssertEqual(f.kind, "prose.delta")
        XCTAssertEqual(f.turnId, "t1")
        XCTAssertEqual(f.seq, 7)
        let payloadStr = String(data: f.payload.data, encoding: .utf8)!
        XCTAssertTrue(payloadStr.contains("\"text\":\"hi\""), "payload should be raw JSON object bytes, got: \(payloadStr)")
    }

    func testGoWireFormatEncodes() throws {
        // Swift→Go: payload must be encoded as literal JSON, never base64.
        let payload = #"{"text":"hi"}"#.data(using: .utf8)!
        let f = Frame(kind: "prose.delta", turnId: "t1", seq: 7, payload: RawJSON(payload))
        let encoded = try FrameCodec.encode(f)
        // Strip 4-byte length prefix and check body
        let body = encoded.suffix(from: 4)
        let str = String(data: body, encoding: .utf8)!
        XCTAssertTrue(str.contains("\"payload\":{\"text\":\"hi\"}"), "got: \(str)")
        XCTAssertFalse(str.contains("base64"))
    }
}

final class IPCClientTests: XCTestCase {
    func testConnectThrowsOnOverlongPath() async {
        // sockaddr_un.sun_path is 104 bytes on Darwin; 200 chars must throw,
        // not leak the fd opened before the length check.
        let longPath = "/tmp/" + String(repeating: "x", count: 200) + ".sock"
        let client = IPCClient(socketPath: longPath)
        do {
            try await client.connect()
            XCTFail("expected pathTooLong error")
        } catch IPCClient.IPCError.pathTooLong {
            // expected
        } catch {
            XCTFail("expected pathTooLong, got \(error)")
        }
    }

    func testConnectThrowsOnMissingSocket() async {
        let client = IPCClient(socketPath: "/tmp/leah-ipc-nonexistent-\(UUID().uuidString).sock")
        do {
            try await client.connect()
            XCTFail("expected connect to fail with no daemon")
        } catch {
            // expected: ECONNREFUSED / ENOENT
        }
    }

    func testRepeatedConnectFailuresDoNotExhaustFDs() async {
        // Each failed connect() must close any fd it opened; otherwise this
        // loop would exhaust the per-process file-descriptor limit (~256).
        let client = IPCClient(socketPath: "/tmp/leah-ipc-nonexistent-\(UUID().uuidString).sock")
        for _ in 0..<500 {
            do { try await client.connect() } catch { /* expected */ }
        }
    }

    func testSendOnUnconnectedClientThrows() async {
        // send() must surface a writeFailed error (EBADF) when fd is unset,
        // never silently succeed. Guards against the nil-baseAddress regression
        // where withUnsafeBytes on empty Data short-circuits to a no-op return.
        let client = IPCClient(socketPath: "/tmp/leah-ipc-unused.sock")
        let f = Frame(kind: "turn.end", turnId: "t1", seq: 1, payload: RawJSON(Data()))
        do {
            try await client.send(f)
            XCTFail("expected writeFailed; send must not silently succeed when fd == -1")
        } catch IPCClient.IPCError.writeFailed {
            // expected: writing to fd=-1 returns EBADF
        } catch {
            XCTFail("expected writeFailed, got \(error)")
        }
    }

    func testDeinitClosesSocketFD() async throws {
        // Actor holds a POSIX fd; without deinit { close } the fd leaks when
        // the client is dropped without an explicit close() call.
        // Verify via socketpair: peer fd sees EOF (read returns 0) once the
        // owning end is closed by deinit.
        var sv: [Int32] = [-1, -1]
        let rc = sv.withUnsafeMutableBufferPointer { ptr in
            Darwin.socketpair(AF_UNIX, SOCK_STREAM, 0, ptr.baseAddress!)
        }
        XCTAssertEqual(rc, 0, "socketpair failed: \(String(cString: strerror(errno)))")
        let ownedFD = sv[0]
        let peerFD = sv[1]
        defer { _ = Darwin.close(peerFD) }

        // Scope the client so it is deallocated on block exit.
        do {
            let client = IPCClient(socketPath: "/unused")
            await client._injectFDForTest(ownedFD)
            _ = client
        }
        // Give the runtime a tick to run actor deinit.
        try await Task.sleep(nanoseconds: 50_000_000)

        var buf = [UInt8](repeating: 0, count: 1)
        let n = Darwin.read(peerFD, &buf, 1)
        XCTAssertEqual(n, 0, "peer must see EOF (0) once owning fd is closed by deinit; got n=\(n) errno=\(errno)")
    }
}
