import Foundation

public struct WidgetGalleryItem: Identifiable {
  public let id: String
  public let kind: WidgetKind
  public let category: WidgetGalleryCategory
  public let name: String
  public let sampleText: String
  private let sampleProps: [String: AnyCodable]

  public init(
    kind: WidgetKind,
    category: WidgetGalleryCategory,
    name: String,
    sampleText: String,
    sampleProps: [String: AnyCodable]
  ) {
    self.id = "gallery-\(kind.rawValue)"
    self.kind = kind
    self.category = category
    self.name = name
    self.sampleText = sampleText
    self.sampleProps = sampleProps
  }

  public func sampleEnvelope() -> WidgetEnvelope {
    WidgetEnvelope(
      widget: kind.rawValue,
      id: id,
      size: WidgetSize.small.rawValue,
      refresh: nil,
      actions: nil,
      props: sampleProps
    )
  }
}

public enum WidgetGalleryCatalog {
  // Live sample envelopes — same shape the LLM emits at spawn time, so the
  // catalog preview is byte-identical to a freshly spawned tile.
  public static let items: [WidgetGalleryItem] = [
    WidgetGalleryItem(
      kind: .market,
      category: .finance,
      name: "Market",
      sampleText: "AAPL price ticker sparkline",
      sampleProps: [
        "symbol": AnyCodable("AAPL"),
        "price": AnyCodable("228.43"),
        "change_pct": AnyCodable("+0.80%"),
        "delta": AnyCodable(0.80),
        "sparkline": AnyCodable([
          AnyCodable(226.1), AnyCodable(226.8), AnyCodable(227.2),
          AnyCodable(227.9), AnyCodable(228.4),
        ]),
      ]
    ),
    WidgetGalleryItem(
      kind: .stat,
      category: .finance,
      name: "Stat",
      sampleText: "52 ▲ +14 / 7d headline numeral",
      sampleProps: [
        "label": AnyCodable("PRS MERGED"),
        "value": AnyCodable("52"),
        "delta": AnyCodable("+14"),
        "period": AnyCodable("7d"),
      ]
    ),
    WidgetGalleryItem(
      kind: .chart,
      category: .finance,
      name: "Chart",
      sampleText: "line area chart series",
      sampleProps: [
        "series": AnyCodable([
          AnyCodable(["x": AnyCodable(0), "y": AnyCodable(12.0)]),
          AnyCodable(["x": AnyCodable(1), "y": AnyCodable(14.2)]),
          AnyCodable(["x": AnyCodable(2), "y": AnyCodable(11.8)]),
          AnyCodable(["x": AnyCodable(3), "y": AnyCodable(15.1)]),
        ]),
      ]
    ),
    WidgetGalleryItem(
      kind: .flights,
      category: .travel,
      name: "Flights",
      sampleText: "SFO JFK flight status departure arrival",
      sampleProps: [
        "flight_number": AnyCodable("UA 555"),
        "origin": AnyCodable("SFO"),
        "destination": AnyCodable("JFK"),
        "departure": AnyCodable("2026-06-22T15:30:00Z"),
        "arrival": AnyCodable("2026-06-22T23:55:00Z"),
        "status": AnyCodable("on time"),
      ]
    ),
    WidgetGalleryItem(
      kind: .maps,
      category: .travel,
      name: "Maps",
      sampleText: "location route directions",
      sampleProps: [
        "query": AnyCodable("Golden Gate Bridge"),
        "latitude": AnyCodable(37.8199),
        "longitude": AnyCodable(-122.4783),
      ]
    ),
    WidgetGalleryItem(
      kind: .weather,
      category: .travel,
      name: "Weather",
      sampleText: "temperature humidity condition",
      sampleProps: [
        "condition": AnyCodable("Partly cloudy"),
        "temp_c": AnyCodable("18"),
        "temp_f": AnyCodable("64"),
        "humidity": AnyCodable("62%"),
        "icon": AnyCodable("cloud.sun"),
        "location": AnyCodable("San Francisco"),
      ]
    ),
    WidgetGalleryItem(
      kind: .calendar,
      category: .time,
      name: "Calendar",
      sampleText: "today events meetings standup",
      sampleProps: [
        "date": AnyCodable("2026-06-22"),
        "events": AnyCodable([
          AnyCodable([
            "title": AnyCodable("Standup"),
            "start": AnyCodable("09:30"),
            "end": AnyCodable("09:45"),
          ]),
        ]),
      ]
    ),
    WidgetGalleryItem(
      kind: .list,
      category: .productivity,
      name: "List",
      sampleText: "todo checklist items",
      sampleProps: [
        "items": AnyCodable([
          AnyCodable("Review PR 395"),
          AnyCodable("Reply to design thread"),
          AnyCodable("Ship gallery overlay"),
        ]),
      ]
    ),
    WidgetGalleryItem(
      kind: .table,
      category: .productivity,
      name: "Table",
      sampleText: "sortable rows columns",
      sampleProps: [
        "columns": AnyCodable([AnyCodable("Name"), AnyCodable("Status")]),
        "rows": AnyCodable([
          AnyCodable([AnyCodable("Task A"), AnyCodable("done")]),
          AnyCodable([AnyCodable("Task B"), AnyCodable("active")]),
        ]),
      ]
    ),
    WidgetGalleryItem(
      kind: .citation,
      category: .web,
      name: "Citation",
      sampleText: "arxiv paper article source URL",
      sampleProps: [
        "url": AnyCodable("https://arxiv.org/abs/2401.00001"),
        "title": AnyCodable("Attention Is All You Need"),
        "site": AnyCodable("arxiv.org"),
      ]
    ),
    WidgetGalleryItem(
      kind: .image,
      category: .web,
      name: "Image",
      sampleText: "photograph picture thumbnail",
      sampleProps: [
        "url": AnyCodable("https://example.com/sample.png"),
        "alt": AnyCodable("sample image"),
      ]
    ),
    WidgetGalleryItem(
      kind: .code,
      category: .code,
      name: "Code",
      sampleText: "go python swift snippet syntax",
      sampleProps: [
        "language": AnyCodable("go"),
        "code": AnyCodable("func main() {\n  println(\"hello\")\n}"),
      ]
    ),
    WidgetGalleryItem(
      kind: .diff,
      category: .code,
      name: "Diff",
      sampleText: "hunk additions deletions file",
      sampleProps: [
        "path": AnyCodable("internal/hud/refresh.go"),
        "hunks": AnyCodable([
          AnyCodable([
            "added": AnyCodable(["return nil"]),
            "removed": AnyCodable(["return err"]),
          ]),
        ]),
      ]
    ),
  ]
}
