import SwiftUI

// SyncPane — Phase 4 §2.9 multi-device sync settings.
//
// Surfaces: peer list, pair-by-OTP entry, per-peer pause/unpair, and the
// "Share API keys via iCloud Keychain" opt-in toggle. The toggle is the
// operator-visible gate that marks API-key Keychain rows synchronizable
// (§2.6 — no raw keys ever transit the peer channel).
//
// The pane is purely a view; all state lives behind SyncIPCClient so the
// daemon owns peer discovery + pairing. Until the daemon-side sync IPC
// frames land, SyncIPCClient is a stub the pane can wire against.

public enum SyncPeerStatus: String, Equatable {
    case online, idle, paused, unreachable

    public var label: String {
        switch self {
        case .online:      return "Online"
        case .idle:        return "Idle"
        case .paused:      return "Paused"
        case .unreachable: return "Unreachable"
        }
    }
}

public struct SyncPeer: Identifiable, Equatable {
    public let id: String
    public let name: String
    public var status: SyncPeerStatus
    public let lastSeenAt: Date?

    public init(id: String, name: String, status: SyncPeerStatus, lastSeenAt: Date? = nil) {
        self.id = id
        self.name = name
        self.status = status
        self.lastSeenAt = lastSeenAt
    }
}

// SyncIPCClient is the pane's view onto sync state. The protocol is the
// seam that lets tests inject deterministic fixtures and lets the daemon
// implementation land independently of the pane.
public protocol SyncIPCClient: AnyObject {
    func listPeers() async throws -> [SyncPeer]
    func pair(otp: String) async throws -> SyncPeer
    func pause(_ peerID: String) async throws
    func resume(_ peerID: String) async throws
    func unpair(_ peerID: String) async throws

    // Reflects the iCloud Keychain "Share API keys" opt-in. The daemon
    // toggles kSecAttrSynchronizable on the existing API-key rows when set.
    func isShareAPIKeysEnabled() async throws -> Bool
    func setShareAPIKeysEnabled(_ on: Bool) async throws
}

// StubSyncIPCClient is the default daemon-less client until §2.4 sync IPC
// frames ship. It is deliberately inert so the pane renders without
// pretending peers exist — operator sees "No peers yet" + the pair affordance.
public final class StubSyncIPCClient: SyncIPCClient {
    public init() {}
    public func listPeers() async throws -> [SyncPeer] { [] }
    public func pair(otp: String) async throws -> SyncPeer {
        throw NSError(domain: "leah.sync", code: -1,
                      userInfo: [NSLocalizedDescriptionKey: "Pairing daemon not yet wired"])
    }
    public func pause(_ peerID: String) async throws {}
    public func resume(_ peerID: String) async throws {}
    public func unpair(_ peerID: String) async throws {}
    public func isShareAPIKeysEnabled() async throws -> Bool { false }
    public func setShareAPIKeysEnabled(_ on: Bool) async throws {}
}

public struct SyncPane: View {
    private let client: SyncIPCClient

    @State private var peers: [SyncPeer] = []
    @State private var otp: String = ""
    @State private var shareKeys: Bool = false
    @State private var pendingUnpair: SyncPeer?
    @State private var lastError: String?

    public init(client: SyncIPCClient = StubSyncIPCClient()) {
        self.client = client
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            peerSection
            Divider()
            pairSection
            Divider()
            shareKeysSection
            if let err = lastError {
                Text(err)
                    .font(.caption)
                    .foregroundColor(.red)
            }
            Spacer()
        }
        .padding(24)
        .task { await refresh() }
        .alert(item: $pendingUnpair) { peer in
            Alert(
                title: Text("Unpair \(peer.name)?"),
                message: Text("Shared keys for this peer are wiped. Pairing again requires a new OTP."),
                primaryButton: .destructive(Text("Unpair")) {
                    Task { await unpair(peer) }
                },
                secondaryButton: .cancel()
            )
        }
    }

    private var peerSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Paired Macs").font(.headline)
            if peers.isEmpty {
                Text("No peers yet.")
                    .foregroundColor(.secondary)
            } else {
                ForEach(peers) { peer in
                    peerRow(peer)
                }
            }
        }
    }

    private func peerRow(_ peer: SyncPeer) -> some View {
        HStack(spacing: 12) {
            Text(peer.name)
            Text(peer.status.label)
                .font(.caption)
                .foregroundColor(.secondary)
            Spacer()
            if peer.status == .paused {
                Button("Resume") { Task { await resume(peer) } }
            } else {
                Button("Pause") { Task { await pause(peer) } }
            }
            Button("Unpair") { pendingUnpair = peer }
                .foregroundColor(.red)
        }
    }

    private var pairSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Pair a new Mac").font(.headline)
            HStack {
                TextField("6-digit OTP", text: $otp)
                    .textFieldStyle(.roundedBorder)
                    .frame(maxWidth: 160)
                Button("Pair") { Task { await pair() } }
                    .disabled(otp.count != 6)
            }
        }
    }

    private var shareKeysSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Toggle("Share API keys via iCloud Keychain", isOn: Binding(
                get: { shareKeys },
                set: { newValue in Task { await toggleShareKeys(newValue) } }
            ))
            Text("API keys never transit the peer channel — they ride iCloud Keychain only when this is on.")
                .font(.caption)
                .foregroundColor(.secondary)
        }
    }

    // MARK: actions

    func refresh() async {
        do {
            peers = try await client.listPeers()
            shareKeys = try await client.isShareAPIKeysEnabled()
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
    }

    func pair() async {
        do {
            let peer = try await client.pair(otp: otp)
            peers.append(peer)
            otp = ""
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
    }

    func pause(_ peer: SyncPeer) async {
        do {
            try await client.pause(peer.id)
            await refresh()
        } catch {
            lastError = error.localizedDescription
        }
    }

    func resume(_ peer: SyncPeer) async {
        do {
            try await client.resume(peer.id)
            await refresh()
        } catch {
            lastError = error.localizedDescription
        }
    }

    func unpair(_ peer: SyncPeer) async {
        do {
            try await client.unpair(peer.id)
            peers.removeAll { $0.id == peer.id }
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
    }

    func toggleShareKeys(_ on: Bool) async {
        do {
            try await client.setShareAPIKeysEnabled(on)
            shareKeys = on
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
    }
}

extension SyncPeer: Hashable {
    public func hash(into hasher: inout Hasher) { hasher.combine(id) }
}
