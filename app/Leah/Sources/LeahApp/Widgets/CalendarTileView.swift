import SwiftUI

public struct CalendarPayload: Equatable {
  public struct Event: Equatable, Identifiable {
    public let id: String
    public let title: String
    public let start: Date
    public let end: Date
    public let isCurrent: Bool
  }

  public let range: String
  public let now: Date
  public let events: [Event]

  public init(range: String, now: Date, events: [Event]) {
    self.range = range; self.now = now; self.events = events
  }

  public enum DecodeError: Error { case missing(String) }

  public init(props: [String: AnyCodable]) throws {
    let range = (props["range"]?.value as? String) ?? "today"
    let fmt = ISO8601DateFormatter()
    let now: Date = {
      if let s = props["now"]?.value as? String, let d = fmt.date(from: s) { return d }
      return Date()
    }()

    guard let arr = props["events"]?.value as? [AnyCodable] else {
      throw DecodeError.missing("events")
    }
    var events: [Event] = []
    for (i, item) in arr.enumerated() {
      guard let obj = item.value as? [String: AnyCodable],
            let id = obj["id"]?.value as? String,
            let title = obj["title"]?.value as? String,
            let startS = obj["start"]?.value as? String,
            let endS = obj["end"]?.value as? String,
            let start = fmt.date(from: startS),
            let end = fmt.date(from: endS) else {
        throw DecodeError.missing("events[\(i)]")
      }
      let isCurrent: Bool = {
        if let b = obj["is_current"]?.value as? Bool { return b }
        return start <= now && now < end
      }()
      events.append(Event(id: id, title: title, start: start, end: end, isCurrent: isCurrent))
    }
    self.init(range: range, now: now, events: events)
  }
}

public struct CalendarTileView: View {
  public let payload: CalendarPayload

  public init(payload: CalendarPayload) { self.payload = payload }

  public static let goldHairlineWidth: CGFloat = 1.5

  public static func nextEvents(payload: CalendarPayload, max limit: Int) -> [CalendarPayload.Event] {
    let upcoming = payload.events
      .filter { $0.end > payload.now }
      .sorted { $0.start < $1.start }
    return Array(upcoming.prefix(limit))
  }

  public static func hasGoldHairline(for event: CalendarPayload.Event) -> Bool {
    event.isCurrent
  }

  public static func nowTickFraction(payload: CalendarPayload) -> Double? {
    let visible = nextEvents(payload: payload, max: 3)
    guard let first = visible.first, let last = visible.last else { return nil }
    let span = last.end.timeIntervalSince(first.start)
    guard span > 0 else { return nil }
    let pos = payload.now.timeIntervalSince(first.start)
    return min(max(pos / span, 0.0), 1.0)
  }

  private static let gold     = Color(red: 0xC9/255.0, green: 0xA9/255.0, blue: 0x61/255.0)
  private static let goldMute = Color(red: 0x8A/255.0, green: 0x73/255.0, blue: 0x40/255.0)
  private static let ivory    = Color(red: 0xF2/255.0, green: 0xED/255.0, blue: 0xE0/255.0)
  private static let textDim  = Color(red: 0x8A/255.0, green: 0x84/255.0, blue: 0x78/255.0)

  private static let timeFmt: DateFormatter = {
    let f = DateFormatter()
    f.dateFormat = "HH:mm"
    return f
  }()

  public var body: some View {
    let visible = Self.nextEvents(payload: payload, max: 3)
    return VStack(alignment: .leading, spacing: 10) {
      Text("CALENDAR \u{00B7} \(payload.range.uppercased())")
        .font(.system(size: 11, weight: .medium))
        .tracking(1.2)
        .foregroundColor(Self.textDim)

      ZStack(alignment: .topLeading) {
        VStack(alignment: .leading, spacing: 8) {
          ForEach(visible) { event in
            HStack(spacing: 10) {
              Rectangle()
                .fill(event.isCurrent ? Self.gold : Color.clear)
                .frame(width: Self.goldHairlineWidth)
                .frame(maxHeight: .infinity)
              VStack(alignment: .leading, spacing: 2) {
                Text(event.title)
                  .font(.system(size: 13, weight: .medium))
                  .foregroundColor(Self.ivory)
                Text("\(Self.timeFmt.string(from: event.start)) \u{2013} \(Self.timeFmt.string(from: event.end))")
                  .font(.system(size: 11))
                  .foregroundColor(Self.textDim)
              }
              Spacer()
            }
            .frame(minHeight: 32)
          }
        }
      }

      GeometryReader { geo in
        ZStack(alignment: .leading) {
          Rectangle()
            .fill(Self.goldMute.opacity(0.4))
            .frame(height: 1)
          if let frac = Self.nowTickFraction(payload: payload) {
            Rectangle()
              .fill(Self.gold)
              .frame(width: 1, height: 8)
              .offset(x: geo.size.width * CGFloat(frac), y: -3)
          }
        }
      }
      .frame(height: 8)
    }
    .padding(12)
  }
}
