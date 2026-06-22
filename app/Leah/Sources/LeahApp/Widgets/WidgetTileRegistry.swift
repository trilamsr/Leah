import SwiftUI

@MainActor
public final class WidgetTileRegistry {
  public typealias Builder = (WidgetEnvelope) -> AnyView

  private var builders: [WidgetKind: Builder] = [:]

  public init() {}

  public func register(kind: WidgetKind, builder: @escaping Builder) {
    builders[kind] = builder
  }

  public func view(for envelope: WidgetEnvelope) -> AnyView? {
    guard let kind = WidgetKind(rawValue: envelope.widget),
          let build = builders[kind] else { return nil }
    return build(envelope)
  }

  public func registerWave2Defaults() {
    register(kind: .flights) { e in
      guard let p: FlightsPayload = decode(e.props) else { return AnyView(EmptyView()) }
      return AnyView(FlightsTileView(payload: p))
    }
    register(kind: .maps) { e in
      guard let p: MapsPayload = decode(e.props) else { return AnyView(EmptyView()) }
      return AnyView(MapsTileView(payload: p))
    }
  }
}

private func decode<T: Decodable>(_ props: [String: AnyCodable]) -> T? {
  let wrapped = props.mapValues { $0 }
  guard let data = try? JSONEncoder().encode(wrapped) else { return nil }
  return try? JSONDecoder().decode(T.self, from: data)
}
