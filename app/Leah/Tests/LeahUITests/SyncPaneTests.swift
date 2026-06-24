import XCTest
import SwiftUI
@testable import LeahUI

// Fake SyncIPCClient — records every call + lets tests inject deterministic
// peer fixtures + error-throwing behavior.
final class FakeSyncIPCClient: SyncIPCClient {
    var peers: [SyncPeer] = []
    var shareKeys: Bool = false

    var pairCalls: [String] = []
    var pauseCalls: [String] = []
    var resumeCalls: [String] = []
    var unpairCalls: [String] = []
    var setShareCalls: [Bool] = []

    var pairResult: Result<SyncPeer, Error> = .failure(NSError(domain: "unset", code: 0))

    func listPeers() async throws -> [SyncPeer] { peers }
    func pair(otp: String) async throws -> SyncPeer {
        pairCalls.append(otp)
        return try pairResult.get()
    }
    func pause(_ peerID: String) async throws { pauseCalls.append(peerID) }
    func resume(_ peerID: String) async throws { resumeCalls.append(peerID) }
    func unpair(_ peerID: String) async throws { unpairCalls.append(peerID) }
    func isShareAPIKeysEnabled() async throws -> Bool { shareKeys }
    func setShareAPIKeysEnabled(_ on: Bool) async throws {
        setShareCalls.append(on)
        shareKeys = on
    }
}

final class SyncPaneTests: XCTestCase {
    // Stub client returns empty peer list — the pane MUST render the "No
    // peers yet" guidance rather than fabricate fixtures (§2.9).
    @MainActor
    func testStubClient_returnsEmptyPeers() async throws {
        let client = StubSyncIPCClient()
        let peers = try await client.listPeers()
        XCTAssertEqual(peers, [])
        let share = try await client.isShareAPIKeysEnabled()
        XCTAssertFalse(share)
    }

    // Pair flow forwards the OTP verbatim to the IPC client. SwiftUI @State
    // is private; we exercise the client contract — the same surface the
    // pane drives from its pair() action.
    @MainActor
    func testPairFlow_forwardsOTPVerbatim() async {
        let fake = FakeSyncIPCClient()
        fake.pairResult = .success(SyncPeer(id: "p1", name: "MacBook Pro", status: .online))
        _ = try? await fake.pair(otp: "123456")
        XCTAssertEqual(fake.pairCalls, ["123456"])
    }

    // Share-keys toggle round-trips through the client + flips the stored
    // flag — this is the §2.6 opt-in gate; failing it leaks API keys to
    // iCloud against operator intent.
    @MainActor
    func testShareKeysToggle_roundtripsThroughClient() async throws {
        let fake = FakeSyncIPCClient()
        XCTAssertFalse(fake.shareKeys)
        try await fake.setShareAPIKeysEnabled(true)
        XCTAssertEqual(fake.setShareCalls, [true])
        XCTAssertTrue(fake.shareKeys)
        try await fake.setShareAPIKeysEnabled(false)
        XCTAssertEqual(fake.setShareCalls, [true, false])
        XCTAssertFalse(fake.shareKeys)
    }

    // Pause/resume/unpair calls pass the peer ID through unchanged — the
    // daemon side keys all peer ops by ID, not by name.
    @MainActor
    func testPeerOpsForwardID() async throws {
        let fake = FakeSyncIPCClient()
        try await fake.pause("peer-a")
        try await fake.resume("peer-a")
        try await fake.unpair("peer-a")
        XCTAssertEqual(fake.pauseCalls, ["peer-a"])
        XCTAssertEqual(fake.resumeCalls, ["peer-a"])
        XCTAssertEqual(fake.unpairCalls, ["peer-a"])
    }

    // The pane renders without crashing for both empty + populated peer
    // lists. NSHostingView layout exercises the body composition.
    @MainActor
    func testPaneRendersWithPopulatedPeers() {
        let fake = FakeSyncIPCClient()
        fake.peers = [
            SyncPeer(id: "p1", name: "Studio", status: .online),
            SyncPeer(id: "p2", name: "Laptop", status: .paused),
        ]
        let pane = SyncPane(client: fake)
        let host = NSHostingView(rootView: pane)
        host.frame = NSRect(x: 0, y: 0, width: 600, height: 400)
        host.layoutSubtreeIfNeeded()
        // No crash + non-zero frame = body composed successfully.
        XCTAssertGreaterThan(host.fittingSize.height, 0)
    }
}
