import Foundation

public struct Frame: Codable, Sendable {
    public let kind: String
    public let turnId: String
    public let seq: UInt64
    public let payload: Data

    enum CodingKeys: String, CodingKey {
        case kind
        case turnId = "turn_id"
        case seq
        case payload
    }

    public init(kind: String, turnId: String, seq: UInt64, payload: Data) {
        self.kind = kind
        self.turnId = turnId
        self.seq = seq
        self.payload = payload
    }
}

public enum FrameCodec {
    public static let maxFrameBytes = 256 * 1024

    public enum CodecError: Error {
        case oversize
        case shortRead
        case badHeader
        case decodeFailed
    }

    // encode returns 4-byte big-endian length prefix + JSON body.
    // Rejects frames whose JSON encoding exceeds maxFrameBytes.
    public static func encode(_ f: Frame) throws -> Data {
        let body = try JSONEncoder().encode(f)
        guard body.count <= maxFrameBytes else { throw CodecError.oversize }
        var out = Data(count: 4)
        let n = UInt32(body.count).bigEndian
        withUnsafeBytes(of: n) { ptr in out.replaceSubrange(0..<4, with: ptr) }
        out.append(body)
        return out
    }

    // decode reads one length-prefixed frame from data, returning the frame
    // and total bytes consumed (header + body). Returns shortRead if data is
    // too small, oversize if the declared length exceeds maxFrameBytes.
    public static func decode(_ data: Data) throws -> (Frame, Int) {
        guard data.count >= 4 else { throw CodecError.shortRead }
        let n = data.prefix(4).withUnsafeBytes { $0.load(as: UInt32.self).bigEndian }
        guard Int(n) <= maxFrameBytes else { throw CodecError.oversize }
        let total = 4 + Int(n)
        guard data.count >= total else { throw CodecError.shortRead }
        let body = data.subdata(in: 4..<total)
        let frame = try JSONDecoder().decode(Frame.self, from: body)
        return (frame, total)
    }
}
