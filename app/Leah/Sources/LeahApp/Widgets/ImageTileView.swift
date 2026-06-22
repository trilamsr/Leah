import SwiftUI
import AppKit

public struct ImageTileView: View {
  public enum LoadState: Equatable { case ok, broken }
  public enum PathError: Error, Equatable { case urlFieldForbidden, cachedPathRequired }

  public let envelope: WidgetEnvelope

  public init(envelope: WidgetEnvelope) { self.envelope = envelope }

  public var body: some View {
    let path = (try? Self.resolvePath(from: envelope)) ?? ""
    let state = Self.loadState(path: path)

    ZStack(alignment: .topLeading) {
      Group {
        if state == .ok, let img = NSImage(contentsOfFile: path) {
          Image(nsImage: img).resizable().aspectRatio(contentMode: .fit)
        } else {
          Color.clear
        }
      }
      .frame(maxWidth: .infinity, maxHeight: .infinity)
      .background(LeahPalette.obsidian2)
      .overlay(
        RoundedRectangle(cornerRadius: 12)
          .strokeBorder(LeahPalette.champagneGold.opacity(0.6), lineWidth: 1)
      )
      .cornerRadius(12)

      if state == .broken {
        Text("BROKEN")
          .font(.system(size: 10, weight: .semibold))
          .tracking(0.5)
          .foregroundColor(LeahPalette.ivory)
          .padding(.horizontal, 6).padding(.vertical, 2)
          .background(LeahPalette.oxblood)
          .cornerRadius(3)
          .padding(8)
      }
    }
  }

  // Hard rule: image tiles MUST load from a daemon-cached path. Any prop key that
  // names a URL field, or any value that parses as http(s)://, is a security bug.
  static func resolvePath(from env: WidgetEnvelope) throws -> String {
    for (k, v) in env.props {
      if k.lowercased() == "url" { throw PathError.urlFieldForbidden }
      if let s = v.value as? String {
        let lower = s.lowercased()
        if lower.hasPrefix("http://") || lower.hasPrefix("https://") {
          throw PathError.urlFieldForbidden
        }
      }
    }
    guard let p = env.props["cached_path"]?.value as? String, !p.isEmpty else {
      throw PathError.cachedPathRequired
    }
    return p
  }

  static func loadState(path: String) -> LoadState {
    FileManager.default.fileExists(atPath: path) ? .ok : .broken
  }

  public static func register(in registry: WidgetTileRegistry) {
    registry.register(kind: .image) { env in AnyView(ImageTileView(envelope: env)) }
  }
}
