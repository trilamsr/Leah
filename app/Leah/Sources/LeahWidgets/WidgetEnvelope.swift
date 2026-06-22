import Foundation

public struct AnyCodable: Codable {
    public let value: Any

    public init(_ value: Any) { self.value = value }

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if let s = try? c.decode(String.self)                { value = s; return }
        if let i = try? c.decode(Int.self)                   { value = i; return }
        if let d = try? c.decode(Double.self)                { value = d; return }
        if let b = try? c.decode(Bool.self)                  { value = b; return }
        if let a = try? c.decode([AnyCodable].self)          { value = a; return }
        if let o = try? c.decode([String: AnyCodable].self)  { value = o; return }
        value = NSNull()
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch value {
        case let s as String:       try c.encode(s)
        case let i as Int:          try c.encode(i)
        case let d as Double:       try c.encode(d)
        case let b as Bool:         try c.encode(b)
        case let a as [AnyCodable]: try c.encode(a)
        case let o as [String: AnyCodable]: try c.encode(o)
        default:                    try c.encodeNil()
        }
    }
}

public struct WidgetEnvelope: Codable {
    public let id: String
    public let widget: String
    public let size: String
    public let props: [String: AnyCodable]
    public let data: [String: AnyCodable]
}
