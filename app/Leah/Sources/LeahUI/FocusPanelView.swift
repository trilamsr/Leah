import SwiftUI
import LeahIPC

public struct FocusPanelView: View {
  // @StateObject keeps the model alive across SwiftUI body re-renders.
  // Production uses the IPCClient overload; tests build the model directly.
  @StateObject private var model: FocusPanelModel
  // Optional level source — present during TTS playback, absent in
  // gallery/preview harnesses (hides the waveform glyph).
  private let levelStream: AsyncStream<Float>?

  public init(client: IPCClient, levelStream: AsyncStream<Float>? = nil) {
    _model = StateObject(wrappedValue: FocusPanelModel(transport: client))
    self.levelStream = levelStream
  }

  public init(model: FocusPanelModel, levelStream: AsyncStream<Float>? = nil) {
    _model = StateObject(wrappedValue: model)
    self.levelStream = levelStream
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 12) {
      HStack {
        Text("⬡")
          .font(.title3.weight(.medium))
          .foregroundColor(Color(red: 201/255, green: 169/255, blue: 97/255))
        TextField("Ask Leah anything…", text: $model.input)
          .textFieldStyle(.plain)
          .font(.system(size: 18))
          .onSubmit {
            let text = model.input
            Task { await model.send(text: text) }
          }
        if !model.levels.isEmpty {
          waveform
            .frame(width: 96, height: 22)
            .accessibilityIdentifier("focus.waveform")
        }
      }
      Divider().background(Color.white.opacity(0.20))
      content
      Spacer()
    }
    .padding(24)
    .background(Color(red: 8/255, green: 9/255, blue: 12/255))
    .task {
      if let stream = levelStream { await model.bind(levelStream: stream) }
    }
    .onChange(of: model.connection) { newValue in
      // Auto-reconnect kicks off while in .error; the loop exits itself once
      // a probe flips connection to .connected.
      if case .error = newValue {
        Task { await model.reconnectLoop() }
      }
    }
  }

  @ViewBuilder
  private var content: some View {
    switch model.connection {
    case .connecting where model.lines.isEmpty:
      emptyState
    case .error(let reason):
      errorState(reason: reason)
    default:
      transcript
    }
  }

  private var emptyState: some View {
    HStack(spacing: 10) {
      ProgressView()
        .controlSize(.small)
        .accessibilityIdentifier("focus.spinner")
      Text("Starting Leah daemon…")
        .font(.system(size: 13))
        .foregroundColor(Color.white.opacity(0.60))
    }
    .frame(maxWidth: .infinity, alignment: .leading)
    .accessibilityIdentifier("focus.empty")
  }

  private func errorState(reason: String) -> some View {
    VStack(alignment: .leading, spacing: 10) {
      Text("Lost connection to Leah. Retrying…")
        .font(.system(size: 13))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Button("Retry now") { Task { await model.retry() } }
        .accessibilityIdentifier("focus.retry")
    }
    .frame(maxWidth: .infinity, alignment: .leading)
    .accessibilityIdentifier("focus.error")
  }

  private var transcript: some View {
    ScrollViewReader { proxy in
      ScrollView {
        VStack(alignment: .leading, spacing: 8) {
          ForEach(model.lines) { line in
            Text(line.text)
              .font(.system(size: 14))
              .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
              .frame(maxWidth: .infinity, alignment: .leading)
              .id(line.id)
          }
          // Invisible footer is the canonical "scrolled to bottom" anchor so
          // bottom-tap and auto-scroll land at the true tail, not the last
          // text line (which may have a trailing partial token).
          Color.clear.frame(height: 1).id("focus.bottom")
        }
        .padding(.bottom, 4)
      }
      // Drag content DOWN = scrolling UP = user wants to read history;
      // flip auto-scroll off so new deltas stop snapping the viewport.
      .simultaneousGesture(
        DragGesture(minimumDistance: 8)
          .onChanged { value in
            if value.translation.height > 0, model.autoScroll {
              model.autoScroll = false
            }
          }
      )
      .overlay(alignment: .bottom) {
        if !model.autoScroll {
          Button {
            model.autoScroll = true
            withAnimation { proxy.scrollTo("focus.bottom", anchor: .bottom) }
          } label: {
            Text("Jump to latest")
              .font(.system(size: 11))
              .foregroundColor(.white)
              .padding(.horizontal, 10)
              .padding(.vertical, 4)
              .background(Color.black.opacity(0.55))
              .clipShape(Capsule())
          }
          .buttonStyle(.plain)
          .accessibilityIdentifier("focus.jumpToLatest")
          .padding(.bottom, 6)
        }
      }
      .onChange(of: model.lines) { _ in
        guard model.autoScroll else { return }
        withAnimation { proxy.scrollTo("focus.bottom", anchor: .bottom) }
      }
    }
  }

  // Inline mini-waveform — LeahUI cannot import LeahVoice without inverting
  // the package layer graph, so the glyph lives here. The level samples come
  // from the injected AsyncStream<Float>; the view itself is just paint.
  private var waveform: some View {
    Canvas { ctx, size in
      let levels = model.levels
      guard !levels.isEmpty else { return }
      let bar = size.width / CGFloat(levels.count)
      for (i, lvl) in levels.enumerated() {
        let clamped = CGFloat(max(0, min(1, lvl)))
        let h = clamped * size.height
        let rect = CGRect(
          x: CGFloat(i) * bar,
          y: (size.height - h) / 2,
          width: bar * 0.8,
          height: h
        )
        ctx.fill(Path(rect), with: .color(Color(red: 201/255, green: 169/255, blue: 97/255)))
      }
    }
  }
}
