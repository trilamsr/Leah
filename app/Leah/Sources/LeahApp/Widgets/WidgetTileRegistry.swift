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
}
