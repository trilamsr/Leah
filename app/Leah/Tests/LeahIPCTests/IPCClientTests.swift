import XCTest
@testable import LeahIPC

final class FrameCodecTests: XCTestCase {
    func testRoundTripFrame() throws {
        let payload = #"{"text":"hi"}"#.data(using: .utf8)!
        let f = Frame(kind: "prose.delta", turnId: "t1", seq: 7, payload: payload)
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
        let huge = Data(repeating: 0x78, count: 300_000)
        let f = Frame(kind: "prose.delta", turnId: "t1", seq: 1, payload: huge)
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
            let f = Frame(kind: kind, turnId: "t-\(kind)", seq: 42, payload: payload)
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
        // Verify the JSON wire format uses "turn_id" not "turnId"
        let payload = Data()
        let f = Frame(kind: "turn.end", turnId: "xyz", seq: 0, payload: payload)
        let encoded = try FrameCodec.encode(f)
        // Skip the 4-byte header to get JSON body
        let jsonBody = encoded.dropFirst(4)
        let jsonObj = try JSONSerialization.jsonObject(with: jsonBody) as! [String: Any]
        XCTAssertNotNil(jsonObj["turn_id"], "wire key must be turn_id")
        XCTAssertNil(jsonObj["turnId"], "camelCase key must not appear on wire")
        XCTAssertEqual(jsonObj["turn_id"] as? String, "xyz")
    }
}
