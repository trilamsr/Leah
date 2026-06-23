import Foundation
import SwiftUI

public enum ConsentChoice: Sendable, Equatable {
    case sendOnce
    case keepLocal
    case alwaysThisSession
}

public final class SessionConsentLedger: @unchecked Sendable {
    private var granted: Set<String> = []
    private let lock = NSLock()

    public init() {}

    public func granted(mode: String) -> Bool {
        lock.lock(); defer { lock.unlock() }
        return granted.contains(mode)
    }

    public func record(mode: String, choice: ConsentChoice) {
        lock.lock(); defer { lock.unlock() }
        switch choice {
        case .alwaysThisSession:
            granted.insert(mode)
        case .sendOnce, .keepLocal:
            break
        }
    }
}

public final class ConsentGate: @unchecked Sendable {
    public typealias Prompter = (String) async -> ConsentChoice

    private let ledger: SessionConsentLedger
    private let prompter: Prompter

    public init(ledger: SessionConsentLedger, prompter: @escaping Prompter) {
        self.ledger = ledger
        self.prompter = prompter
    }

    public func request(mode: String) async -> Bool {
        if ledger.granted(mode: mode) { return true }
        let choice = await prompter(mode)
        ledger.record(mode: mode, choice: choice)
        switch choice {
        case .sendOnce, .alwaysThisSession:
            return true
        case .keepLocal:
            return false
        }
    }
}

public struct ConsentSheet: View {
    public let mode: String
    public let onChoice: (ConsentChoice) -> Void

    public init(mode: String, onChoice: @escaping (ConsentChoice) -> Void) {
        self.mode = mode
        self.onChoice = onChoice
    }

    public static func title(forMode mode: String) -> String {
        switch mode {
        case "screenshot": return "Send this screenshot to Claude?"
        case "live_screen": return "Stream your screen to Claude?"
        case "live_camera": return "Stream your camera to Claude?"
        default: return "Send this frame to Claude?"
        }
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(Self.title(forMode: mode))
                .font(.headline)
            Text("Frames are uploaded only after you confirm. OCR + local vision never leave this Mac.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            HStack {
                Button("Keep local") { onChoice(.keepLocal) }
                    .keyboardShortcut(.cancelAction)
                Spacer()
                Button("Always this session") { onChoice(.alwaysThisSession) }
                Button("Send") { onChoice(.sendOnce) }
                    .keyboardShortcut(.defaultAction)
            }
        }
        .padding(20)
        .frame(width: 380)
    }
}
