import Foundation

// Mirrors Go's json.RawMessage: encodes/decodes as a literal JSON value, not base64.
public struct RawJSON: Sendable, Equatable {
    public let data: Data

    public init(_ data: Data) { self.data = data }
}

extension RawJSON: Codable {
    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let value = try container.decode(JSONValue.self)
        self.data = try JSONEncoder().encode(value)
    }

    public func encode(to encoder: Encoder) throws {
        let value = try JSONDecoder().decode(JSONValue.self, from: data)
        var container = encoder.singleValueContainer()
        try container.encode(value)
    }
}

public indirect enum JSONValue: Codable, Sendable, Equatable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null; return }
        if let b = try? c.decode(Bool.self) { self = .bool(b); return }
        if let n = try? c.decode(Double.self) { self = .number(n); return }
        if let s = try? c.decode(String.self) { self = .string(s); return }
        if let a = try? c.decode([JSONValue].self) { self = .array(a); return }
        if let o = try? c.decode([String: JSONValue].self) { self = .object(o); return }
        throw DecodingError.dataCorruptedError(in: c, debugDescription: "unknown JSON value")
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .null: try c.encodeNil()
        case .bool(let b): try c.encode(b)
        case .number(let n): try c.encode(n)
        case .string(let s): try c.encode(s)
        case .array(let a): try c.encode(a)
        case .object(let o): try c.encode(o)
        }
    }
}

public struct Frame: Sendable {
    public let kind: String
    public let turnId: String
    public let seq: UInt64
    public let payload: RawJSON

    public init(kind: String, turnId: String, seq: UInt64, payload: RawJSON) {
        self.kind = kind
        self.turnId = turnId
        self.seq = seq
        self.payload = payload
    }
}

extension Frame: Codable {
    enum CodingKeys: String, CodingKey {
        case kind
        case turnId = "turn_id"
        case seq
        case payload
    }

    // Mirrors Go's `json.RawMessage` with omitempty: empty payload → no field on wire.
    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(kind, forKey: .kind)
        try c.encode(turnId, forKey: .turnId)
        try c.encode(seq, forKey: .seq)
        if !payload.data.isEmpty {
            try c.encode(payload, forKey: .payload)
        }
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.kind = try c.decode(String.self, forKey: .kind)
        self.turnId = try c.decode(String.self, forKey: .turnId)
        self.seq = try c.decode(UInt64.self, forKey: .seq)
        self.payload = try c.decodeIfPresent(RawJSON.self, forKey: .payload) ?? RawJSON(Data())
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

    public static func encode(_ f: Frame) throws -> Data {
        let body = try JSONEncoder().encode(f)
        guard body.count <= maxFrameBytes else { throw CodecError.oversize }
        var out = Data(count: 4)
        let n = UInt32(body.count).bigEndian
        withUnsafeBytes(of: n) { ptr in out.replaceSubrange(0..<4, with: ptr) }
        out.append(body)
        return out
    }

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
