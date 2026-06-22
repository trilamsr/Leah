import Foundation

public enum WidgetKind: String, Codable, CaseIterable {
  case market, flights, calendar, weather, maps, table, chart, image, code, citation, stat, list, diff
}

public enum WidgetSize: String, Codable, CaseIterable {
  case small, medium, large, hero
}

public struct AnyCodable: Codable {
  public let value: Any
  public init(_ value: Any) { self.value = value }

  public init(from decoder: Decoder) throws {
    let c = try decoder.singleValueContainer()
    if c.decodeNil()                                     { value = NSNull(); return }
    if let b = try? c.decode(Bool.self)                  { value = b; return }
    if let i = try? c.decode(Int.self)                   { value = i; return }
    if let d = try? c.decode(Double.self)                { value = d; return }
    if let s = try? c.decode(String.self)                { value = s; return }
    if let a = try? c.decode([AnyCodable].self)          { value = a; return }
    if let o = try? c.decode([String: AnyCodable].self)  { value = o; return }
    value = NSNull()
  }

  public func encode(to encoder: Encoder) throws {
    var c = encoder.singleValueContainer()
    switch value {
    case is NSNull:                       try c.encodeNil()
    case let b as Bool:                   try c.encode(b)
    case let i as Int:                    try c.encode(i)
    case let d as Double:                 try c.encode(d)
    case let s as String:                 try c.encode(s)
    case let a as [AnyCodable]:           try c.encode(a)
    case let o as [String: AnyCodable]:   try c.encode(o)
    default:                              try c.encodeNil()
    }
  }
}

public struct WidgetEnvelope: Codable {
  public static let maxEnvelopeBytes = 256 * 1024

  public struct Action: Codable, Equatable {
    public let label: String
    public let callback: String
    public let icon: String?
    public init(label: String, callback: String, icon: String?) {
      self.label = label; self.callback = callback; self.icon = icon
    }
  }

  public let widget: String
  public let id: String
  public let size: String
  public let refresh: Int?
  public let actions: [Action]?
  public let props: [String: AnyCodable]

  public init(
    widget: String,
    id: String,
    size: String,
    refresh: Int? = nil,
    actions: [Action]? = nil,
    props: [String: AnyCodable] = [:]
  ) {
    self.widget = widget; self.id = id; self.size = size
    self.refresh = refresh; self.actions = actions; self.props = props
  }

  public enum ValidationError: Error, CustomStringConvertible {
    case unknownKind(String)
    case unknownSize(String)
    case idEmpty
    case idPattern(String)
    case refreshBelowMinimum(Int)
    case tooManyActions(Int)
    case actionMissingLabelOrCallback(Int)
    case actionLabelTooLong(Int, Int)
    case actionCallbackPattern(Int, String)
    case envelopeTooLarge(Int)

    public var description: String {
      switch self {
      case .unknownKind(let k):              return "unknown widget kind \"\(k)\""
      case .unknownSize(let s):              return "unknown size \"\(s)\""
      case .idEmpty:                         return "id required"
      case .idPattern(let id):               return "id \"\(id)\" must match ^[a-z0-9_-]{1,64}$"
      case .refreshBelowMinimum(let r):      return "refresh \(r) below minimum 5s"
      case .tooManyActions(let n):           return "actions \(n) exceeds max 4"
      case .actionMissingLabelOrCallback(let i): return "action[\(i)] missing label or callback"
      case .actionLabelTooLong(let i, let n): return "action[\(i)] label \(n) chars exceeds max 32"
      case .actionCallbackPattern(let i, let cb): return "action[\(i)] callback \"\(cb)\" must match leah://action/<name>"
      case .envelopeTooLarge(let n):         return "envelope \(n) bytes exceeds 256 KB cap"
      }
    }
  }

  // Mirrors internal/widget/envelope.go regexes verbatim.
  private static let idRegex = try! NSRegularExpression(pattern: "^[a-z0-9_-]{1,64}$")
  private static let callbackRegex = try! NSRegularExpression(pattern: "^leah://action/[a-z_]+(\\?.*)?$")

  private static func matches(_ regex: NSRegularExpression, _ s: String) -> Bool {
    let range = NSRange(s.startIndex..., in: s)
    return regex.firstMatch(in: s, range: range) != nil
  }

  public func validate() throws {
    guard WidgetKind(rawValue: widget) != nil else { throw ValidationError.unknownKind(widget) }
    guard WidgetSize(rawValue: size) != nil else { throw ValidationError.unknownSize(size) }
    if id.isEmpty { throw ValidationError.idEmpty }
    if !Self.matches(Self.idRegex, id) { throw ValidationError.idPattern(id) }
    if let r = refresh, r < 5 { throw ValidationError.refreshBelowMinimum(r) }
    if let acts = actions {
      if acts.count > 4 { throw ValidationError.tooManyActions(acts.count) }
      for (i, a) in acts.enumerated() {
        if a.label.isEmpty || a.callback.isEmpty {
          throw ValidationError.actionMissingLabelOrCallback(i)
        }
        if a.label.count > 32 {
          throw ValidationError.actionLabelTooLong(i, a.label.count)
        }
        if !Self.matches(Self.callbackRegex, a.callback) {
          throw ValidationError.actionCallbackPattern(i, a.callback)
        }
      }
    }
    let encoded = try JSONEncoder().encode(self)
    if encoded.count > Self.maxEnvelopeBytes {
      throw ValidationError.envelopeTooLarge(encoded.count)
    }
  }
}
