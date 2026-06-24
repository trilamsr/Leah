import SwiftUI

public struct VoicePane: View {
    public static let ttsProviderKey = "leah.voice.ttsProvider"
    public static let wakeWordKey = "leah.voice.wakeWord"
    public static let ttsProviderOptions = ["ElevenLabs", "Apple Ava"]
    public static let ttsPhase3Tooltip = "Phase 3 lands runtime"
    public static let ttsRuntimeEnabled = false

    public static func currentTTSProvider() -> String {
        UserDefaults.standard.string(forKey: ttsProviderKey) ?? ttsProviderOptions[0]
    }

    public static func setTTSProvider(_ provider: String) {
        UserDefaults.standard.set(provider, forKey: ttsProviderKey)
    }

    public static func wakeWordEnabled() -> Bool {
        UserDefaults.standard.bool(forKey: wakeWordKey)
    }

    public static func setWakeWordEnabled(_ on: Bool) {
        UserDefaults.standard.set(on, forKey: wakeWordKey)
    }

    @State private var provider: String = VoicePane.currentTTSProvider()
    @State private var wakeWord: Bool = VoicePane.wakeWordEnabled()
    @State private var paneState: PaneState

    public init(initialState: PaneState = .loaded) {
        _paneState = State(initialValue: initialState)
    }

    public var body: some View {
        switch paneState {
        case .empty:
            EmptyState(symbol: "waveform",
                       title: "Voice not configured",
                       caption: "TTS provider + wake-word controls appear once the voice runtime is enabled.")
        case .error(let msg):
            ErrorState(message: msg) { paneState = .loaded }
        case .loaded:
            loaded
        }
    }

    private var loaded: some View {
        VStack(alignment: .leading, spacing: 16) {
            Group {
                Text("TTS provider").font(.system(size: 13, weight: .medium)).foregroundColor(.gray)
                Picker("", selection: $provider) {
                    ForEach(Self.ttsProviderOptions, id: \.self) { Text($0).tag($0) }
                }
                .pickerStyle(.segmented)
                .disabled(!Self.ttsRuntimeEnabled)
                .help(Self.ttsPhase3Tooltip)
                .onChange(of: provider) { _, newValue in Self.setTTSProvider(newValue) }
            }
            Divider()
            Toggle("Listen for \"Hey Leah\" wake-word", isOn: $wakeWord)
                .foregroundColor(.white)
                .onChange(of: wakeWord) { _, newValue in Self.setWakeWordEnabled(newValue) }
            Divider()
            Button("Preview voice canon") {}
                .disabled(true)
                .help(Self.ttsPhase3Tooltip)
            Spacer()
        }
        .padding(24)
    }
}
