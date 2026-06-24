import SwiftUI
import EventKit
import AppKit

// Step 4: Connect integrations. Each row shells out to `leah connect <name>`
// (device-code OAuth opens the browser); Calendar uses native EventKit consent
// because there is no OAuth provider for the local Mac calendar.
// "Connect later" silently advances — wizard never blocks on optional auth.
public struct IntegrationStep: View {
  let onContinue: () -> Void
  // Test seam: replaced in unit tests to avoid spawning `leah` binary.
  let runConnect: (String) -> Bool

  @State private var status: [Integration: ConnectionState] = [:]
  @State private var lastMessage = ""
  private let store = EKEventStore()

  public init(
    onContinue: @escaping () -> Void,
    runConnect: @escaping (String) -> Bool = IntegrationStep.shellConnect
  ) {
    self.onContinue = onContinue
    self.runConnect = runConnect
  }

  public enum Integration: String, CaseIterable, Equatable {
    case calendar = "Calendar"
    case mail     = "Mail"
    case files    = "Files"

    // CLI name passed to `leah connect <name>`. Calendar is native-consent only.
    var connectArg: String? {
      switch self {
      case .calendar: return nil
      case .mail:     return "gmail"
      case .files:    return nil
      }
    }
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Connect your data.")
        .font(.title2.weight(.medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Pick any. You can always connect more later in Settings → Connections.")
        .font(.callout)
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      ForEach(Integration.allCases, id: \.self) { opt in
        row(for: opt)
      }
      if !lastMessage.isEmpty {
        Text(lastMessage).font(.caption).foregroundColor(.gray)
      }
      Spacer()
      HStack {
        Button("Connect later", action: onContinue)
          .buttonStyle(.plain)
          .foregroundColor(.secondary)
        Spacer()
        Button("Continue", action: onContinue)
      }
    }
    .padding(48)
  }

  @ViewBuilder
  private func row(for opt: Integration) -> some View {
    HStack(spacing: 10) {
      StatusGlyphView(state: status[opt] ?? .none)
      Text(opt.rawValue)
        .font(.callout)
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      Spacer()
      Button(status[opt] == .connected ? "Connected" : "Connect now (opens browser)") {
        connect(opt)
      }
      .disabled(status[opt] == .connected)
      .controlSize(.small)
    }
  }

  private func connect(_ opt: Integration) {
    switch opt {
    case .calendar:
      connectCalendar()
    case .mail, .files:
      guard let arg = opt.connectArg else {
        // No CLI mapping yet (Files) — silent skip, no error toast per spec.
        return
      }
      let ok = runConnect(arg)
      if ok {
        status[opt] = .connected
        lastMessage = "\(opt.rawValue) connected."
      }
      // Cancelled / error: silent. User can retry from Settings → Connections.
    }
  }

  private func connectCalendar() {
    let handler: (Bool, Error?) -> Void = { ok, _ in
      DispatchQueue.main.async {
        if ok {
          status[.calendar] = .connected
          lastMessage = "Calendar connected."
        }
      }
    }
    if #available(macOS 14.0, *) {
      store.requestFullAccessToEvents(completion: handler)
    } else {
      store.requestAccess(to: .event, completion: handler)
    }
  }

  // Locates the leah binary next to the running app (Contents/Resources/leah
  // in a signed bundle) or on PATH for dev builds, then runs `leah connect
  // <name>`. Returns true on exit 0; any non-zero is treated as cancel/fail
  // and the caller stays silent.
  public static func shellConnect(_ name: String) -> Bool {
    let url = locateLeahBinary()
    let p = Process()
    p.executableURL = url
    p.arguments = ["connect", name]
    do {
      try p.run()
      p.waitUntilExit()
      return p.terminationStatus == 0
    } catch {
      return false
    }
  }

  private static func locateLeahBinary() -> URL {
    // Bundled binary (release path).
    if let res = Bundle.main.url(forResource: "leah", withExtension: nil) {
      return res
    }
    // Common dev locations relative to the executable.
    let exe = Bundle.main.executableURL ?? URL(fileURLWithPath: "/usr/local/bin/leah")
    let bin = exe.deletingLastPathComponent().appendingPathComponent("leah")
    if FileManager.default.isExecutableFile(atPath: bin.path) {
      return bin
    }
    return URL(fileURLWithPath: "/usr/local/bin/leah")
  }
}
