import XCTest
@testable import LeahUI

final class ConnectionsPaneA2ATests: XCTestCase {
    func testPeerListRoundtripsThroughIPCClient() {
        let client = PreviewA2AIPCClient()
        let id = client.addStub(name: "studio-mac", lastSeen: Date(timeIntervalSince1970: 1_700_000_000))
        let peers = client.listPeers()
        XCTAssertEqual(peers.count, 1)
        XCTAssertEqual(peers[0].id, id)
        XCTAssertEqual(peers[0].name, "studio-mac")
        XCTAssertEqual(peers[0].status, .active)
    }

    func testPauseFlipsStatusAndUnpairRemoves() {
        let client = PreviewA2AIPCClient()
        let id = client.addStub(name: "studio-mac")
        client.pause(id: id)
        XCTAssertEqual(client.listPeers().first?.status, .paused)
        client.unpair(id: id)
        XCTAssertTrue(client.listPeers().isEmpty)
    }

    // §5.4 mandates 6-digit numeric OTP rendered NNN-NNN; pairStart must reject
    // anything else so the daemon IPC never sees malformed input.
    func testPairStartRejectsMalformedOTP() {
        let client = PreviewA2AIPCClient()
        XCTAssertNil(client.pairStart(otp: "12345"))
        XCTAssertNil(client.pairStart(otp: "abcdef"))
        XCTAssertNil(client.pairStart(otp: ""))
        XCTAssertNotNil(client.pairStart(otp: "482-913"))
        XCTAssertNotNil(client.pairStart(otp: "482913"))
    }

    func testAutoAcceptToggleIsClientLocal() {
        let client = PreviewA2AIPCClient()
        XCTAssertFalse(client.autoAcceptSameNet)
        client.autoAcceptSameNet = true
        XCTAssertTrue(client.autoAcceptSameNet)
    }
}
