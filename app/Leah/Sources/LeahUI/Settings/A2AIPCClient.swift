import Foundation

// A2APeerStatus mirrors the Paused flag on Go a2a.A2APeer (server.go §5.4.1).
// `.active` == paired & not paused; `.paused` == operator-suspended.
public enum A2APeerStatus: String, Equatable {
    case active, paused
}

// A2APeerRow is the Swift-side projection of Go a2a.A2APeer. `id` is the
// hex-encoded Ed25519 fingerprint (PeerID.String()); only the rendered
// fields cross the IPC seam.
public struct A2APeerRow: Identifiable, Equatable {
    public let id: String
    public let name: String
    public let lastSeen: Date
    public var status: A2APeerStatus

    public init(id: String, name: String, lastSeen: Date, status: A2APeerStatus) {
        self.id = id
        self.name = name
        self.lastSeen = lastSeen
        self.status = status
    }
}

// A2AIPCClient is the dep-inversion seam between the Settings pane and the
// daemon's a2a kinds (`a2a.peer.list`, `a2a.pair.start`, `a2a.peer.pause`,
// `a2a.peer.unpair`). Production wires the live IPC client; previews + tests
// use PreviewA2AIPCClient so the UI is exercised without a daemon.
public protocol A2AIPCClient: AnyObject {
    var autoAcceptSameNet: Bool { get set }
    func listPeers() -> [A2APeerRow]
    func pairStart(otp: String) -> String?
    func pause(id: String)
    func unpair(id: String)
}

// PreviewA2AIPCClient holds peers in-memory and is the only implementation
// callers see in the absence of a wired daemon. OTP validation matches §5.4:
// 6 digits, optionally rendered NNN-NNN; any non-digit, non-dash char fails.
public final class PreviewA2AIPCClient: A2AIPCClient {
    private var peers: [A2APeerRow] = []
    public var autoAcceptSameNet: Bool = false

    public init() {}

    public func listPeers() -> [A2APeerRow] { peers }

    @discardableResult
    public func addStub(name: String, lastSeen: Date = Date()) -> String {
        let id = String(format: "%016x", UInt64.random(in: 0...UInt64.max))
        peers.append(A2APeerRow(id: id, name: name, lastSeen: lastSeen, status: .active))
        return id
    }

    public func pairStart(otp: String) -> String? {
        guard Self.isValidOTP(otp) else { return nil }
        return addStub(name: "peer-\(otp.suffix(3))")
    }

    public func pause(id: String) {
        for i in peers.indices where peers[i].id == id {
            peers[i].status = peers[i].status == .paused ? .active : .paused
        }
    }

    public func unpair(id: String) {
        peers.removeAll { $0.id == id }
    }

    static func isValidOTP(_ s: String) -> Bool {
        let digits = s.filter { $0 != "-" }
        guard digits.count == 6 else { return false }
        return digits.allSatisfy { $0.isASCII && $0.isNumber }
    }
}
