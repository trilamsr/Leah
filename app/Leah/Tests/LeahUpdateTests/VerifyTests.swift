import XCTest
@testable import LeahUpdate

final class VerifyTests: XCTestCase {
    // Sparkle's own EdDSAVerifier runs first during install; this redundant
    // verify is belt-and-suspenders, used before rollback exposes a prior
    // build so the operator can't roll-forward to an unsigned channel item.

    private func tmp(_ name: String) -> URL {
        URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("leah-verifytests-\(UUID().uuidString)-\(name)")
    }

    func test_missingZip_returnsFalse() {
        let v = EdDSAVerifier(publicKey: testPubKey)
        XCTAssertFalse(v.verify(zipPath: tmp("absent.zip"), signature: "abc"))
    }

    func test_emptySignature_returnsFalse() throws {
        let path = tmp("empty-sig.zip")
        try Data("payload".utf8).write(to: path)
        defer { try? FileManager.default.removeItem(at: path) }
        let v = EdDSAVerifier(publicKey: testPubKey)
        XCTAssertFalse(v.verify(zipPath: path, signature: ""))
    }

    func test_emptyPublicKey_returnsFalse() throws {
        let path = tmp("empty-key.zip")
        try Data("payload".utf8).write(to: path)
        defer { try? FileManager.default.removeItem(at: path) }
        let v = EdDSAVerifier(publicKey: "")
        XCTAssertFalse(v.verify(zipPath: path, signature: validBase64Sig))
    }

    func test_malformedSignature_returnsFalse() throws {
        let path = tmp("bad-sig.zip")
        try Data("payload".utf8).write(to: path)
        defer { try? FileManager.default.removeItem(at: path) }
        let v = EdDSAVerifier(publicKey: testPubKey)
        XCTAssertFalse(v.verify(zipPath: path, signature: "not-base64-!!!"))
    }

    func test_signatureWrongLength_returnsFalse() throws {
        let path = tmp("wrong-len.zip")
        try Data("payload".utf8).write(to: path)
        defer { try? FileManager.default.removeItem(at: path) }
        let v = EdDSAVerifier(publicKey: testPubKey)
        // Valid base64, but decodes to 8 bytes — Ed25519 sigs are 64.
        XCTAssertFalse(v.verify(zipPath: path, signature: Data(repeating: 0, count: 8).base64EncodedString()))
    }

    func test_corruptedPayload_returnsFalse() throws {
        let path = tmp("corrupt.zip")
        try Data("not-the-payload-that-was-signed".utf8).write(to: path)
        defer { try? FileManager.default.removeItem(at: path) }
        let v = EdDSAVerifier(publicKey: testPubKey)
        XCTAssertFalse(v.verify(zipPath: path, signature: validBase64Sig))
    }

    // A real EdDSA sign-and-verify round trip on a temp keypair proves the
    // verifier is wired to a real crypto check, not a length-only stub.
    func test_validSignature_returnsTrue() throws {
        let payload = Data("leah-test-payload".utf8)
        let path = tmp("ok.zip")
        try payload.write(to: path)
        defer { try? FileManager.default.removeItem(at: path) }

        let kp = try EdDSAKeyPair.generate()
        let sig = try kp.sign(payload).base64EncodedString()
        let v = EdDSAVerifier(publicKey: kp.publicKeyBase64)
        XCTAssertTrue(v.verify(zipPath: path, signature: sig))
    }

    func test_validSignatureButWrongKey_returnsFalse() throws {
        let payload = Data("leah-test-payload".utf8)
        let path = tmp("ok.zip")
        try payload.write(to: path)
        defer { try? FileManager.default.removeItem(at: path) }

        let signer = try EdDSAKeyPair.generate()
        let other = try EdDSAKeyPair.generate()
        let sig = try signer.sign(payload).base64EncodedString()
        let v = EdDSAVerifier(publicKey: other.publicKeyBase64)
        XCTAssertFalse(v.verify(zipPath: path, signature: sig))
    }

    // Test fixtures — `validBase64Sig` is structurally valid base64 of a
    // 64-byte zero buffer, used to isolate the "wrong key" / "wrong payload"
    // branches from the "malformed input" branch.
    private let testPubKey = "MCowBQYDK2VwAyEAEC1nDr/CqzgJ7XBndlnHkP+JONIWcCN8e7+rzGYIO+M="
    private let validBase64Sig = Data(repeating: 0, count: 64).base64EncodedString()
}

final class ChannelTests: XCTestCase {
    // The rollback channel exists for the case where a freshly notarized
    // build regresses — the operator flips a toggle and Sparkle starts
    // honoring `<sparkle:channel>rollback</sparkle:channel>` items that
    // point at the prior known-good build.
    func test_default_allowsStableOnly() {
        XCTAssertEqual(UpdateChannelPolicy(useRollback: false).allowedChannels, ["stable"])
    }

    func test_rollbackOptIn_addsRollbackChannel() {
        let p = UpdateChannelPolicy(useRollback: true)
        XCTAssertTrue(p.allowedChannels.contains("rollback"))
        XCTAssertTrue(p.allowedChannels.contains("stable"))
    }

    func test_appcastItem_supportsChannelTag() {
        let item = AppcastTemplate.Item(
            version: "1.2.3",
            downloadURL: "https://example.test/Leah-1.2.3.zip",
            length: 12345,
            eddsaSignature: "sigbytes",
            channel: "rollback"
        )
        let xml = AppcastTemplate.item(item)
        XCTAssertTrue(xml.contains("<sparkle:channel>rollback</sparkle:channel>"))
    }

    func test_appcastItem_defaultsToStableChannel() {
        let item = AppcastTemplate.Item(
            version: "1.2.3",
            downloadURL: "https://example.test/Leah-1.2.3.zip",
            length: 12345,
            eddsaSignature: "sigbytes"
        )
        let xml = AppcastTemplate.item(item)
        XCTAssertTrue(xml.contains("<sparkle:channel>stable</sparkle:channel>"))
    }
}
