import SwiftUI
import LeahIPC
import LeahUI

// Host wires the three spec §10.4 spawn affordances (`+` button next to the
// input, `⌘⇧W` shortcut, `/widgets` slash-command) into the Phase 1
// FocusPanelView without modifying its module-internal layout. The gallery
// overlay covers the response area; the input row stays visible at top.
public struct FocusPanelGalleryHost: View {
  let client: IPCClient
  @StateObject private var model = WidgetGalleryModel()
  @State private var slashProbe: String = ""
  let registry: WidgetTileRegistry

  public init(client: IPCClient, registry: WidgetTileRegistry) {
    self.client = client
    self.registry = registry
  }

  public var body: some View {
    ZStack(alignment: .topLeading) {
      VStack(spacing: 0) {
        HStack(spacing: 8) {
          PlusButton { model.openFromPlusButton() }
          FocusPanelView(client: client)
        }
        TextField("", text: $slashProbe)
          .frame(width: 0, height: 0)
          .opacity(0)
          .onChange(of: slashProbe) { newValue in
            if model.consumeSlashIfPresent(newValue) {
              slashProbe = ""
            }
          }
      }
      if model.isVisible {
        WidgetGalleryView(model: model, registry: registry)
          .transition(.opacity)
      }
    }
  }
}
