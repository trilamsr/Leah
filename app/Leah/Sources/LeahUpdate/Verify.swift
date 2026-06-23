import CryptoKit
import Foundation

// EdDSAVerifier reproduces Sparkle's appcast-signature check against the
// downloaded payload. Sparkle's SPUUpdater already runs an equivalent check
// before install; this redundant pass is invoked by the rollback path so the
// operator can't be coerced into reinstalling an unsigned channel item.
// The public key is embedded at compile time (LeahUpdate.embeddedPublicKey),
// never fetched over the network — a compromised appcast cannot swap the key.
public struct EdDSAVerifier {
    private let publicKey: String

    public init(publicKey: String) {
        self.publicKey = publicKey
    }

    public func verify(zipPath: URL, signature: String) -> Bool {
        guard !publicKey.isEmpty, !signature.isEmpty else { return false }
        guard let payload = try? Data(contentsOf: zipPath), !payload.isEmpty else { return false }
        guard let sigData = Data(base64Encoded: signature), sigData.count == 64 else { return false }
        guard let keyData = Data(base64Encoded: publicKey) else { return false }

        // Sparkle's `sign_update` emits a raw 32-byte Ed25519 public key
        // (base64). If the operator pastes a DER/PEM-wrapped key, strip the
        // 12-byte SubjectPublicKeyInfo prefix that wraps the raw point.
        let raw: Data
        if keyData.count == 32 {
            raw = keyData
        } else if keyData.count == 44 {
            raw = keyData.suffix(32)
        } else {
            return false
        }

        guard let pub = try? Curve25519.Signing.PublicKey(rawRepresentation: raw) else { return false }
        return pub.isValidSignature(sigData, for: payload)
    }
}

// Test seam — production code reads the embedded key resource at runtime.
public enum EdDSAKeyPair {
    public struct Pair {
        public let privateKey: Curve25519.Signing.PrivateKey
        public var publicKeyBase64: String { privateKey.publicKey.rawRepresentation.base64EncodedString() }
        public func sign(_ data: Data) throws -> Data { try privateKey.signature(for: data) }
    }
    public static func generate() throws -> Pair { Pair(privateKey: Curve25519.Signing.PrivateKey()) }
}

// UpdateChannelPolicy maps the "Use rollback channel" toggle to the
// `<sparkle:channel>` allow-list Sparkle filters appcast items against.
// Default stable-only; toggling rollback adds it without removing stable, so
// an operator pinned to a rollback build still surfaces forward-fix releases.
public struct UpdateChannelPolicy: Equatable {
    public let allowedChannels: [String]

    public init(useRollback: Bool) {
        self.allowedChannels = useRollback ? ["stable", "rollback"] : ["stable"]
    }
}
