import Foundation
import Darwin

// IPCClient connects to the daemon's AF_UNIX socket and exchanges
// length-prefixed JSON frames (spec §10.7). Call connect() before
// send/receive. Reconnect with backoff is handled by reconnectLoop().
public actor IPCClient {
    private let socketPath: String
    private var fd: Int32 = -1

    public init(socketPath: String = IPCClient.defaultSocketPath) {
        self.socketPath = socketPath
    }

    public static var defaultSocketPath: String {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        return "\(home)/Library/Caches/Leah/leah.sock"
    }

    public func connect() throws {
        let newFD = socket(AF_UNIX, SOCK_STREAM, 0)
        guard newFD >= 0 else { throw POSIXError(.EIO) }
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = Array(socketPath.utf8) + [0]
        precondition(
            pathBytes.count <= MemoryLayout.size(ofValue: addr.sun_path),
            "socket path too long for sockaddr_un"
        )
        withUnsafeMutableBytes(of: &addr.sun_path) { dst in
            pathBytes.withUnsafeBytes { src in dst.copyMemory(from: src) }
        }
        let rc = withUnsafePointer(to: &addr) { ptr -> Int32 in
            ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.connect(newFD, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        guard rc == 0 else {
            _ = Darwin.close(newFD)
            throw POSIXError(.ECONNREFUSED)
        }
        fd = newFD
    }

    public func send(_ f: Frame) throws {
        let bytes = try FrameCodec.encode(f)
        bytes.withUnsafeBytes { ptr in
            _ = write(fd, ptr.baseAddress, bytes.count)
        }
    }

    public func receive() throws -> Frame {
        var hdr = [UInt8](repeating: 0, count: 4)
        let r1 = Darwin.read(fd, &hdr, 4)
        guard r1 == 4 else { throw FrameCodec.CodecError.shortRead }
        let n = UInt32(hdr[0]) << 24 | UInt32(hdr[1]) << 16 | UInt32(hdr[2]) << 8 | UInt32(hdr[3])
        guard Int(n) <= FrameCodec.maxFrameBytes else { throw FrameCodec.CodecError.oversize }
        var body = [UInt8](repeating: 0, count: Int(n))
        let r2 = Darwin.read(fd, &body, Int(n))
        guard r2 == Int(n) else { throw FrameCodec.CodecError.shortRead }
        return try JSONDecoder().decode(Frame.self, from: Data(body))
    }

    public func close() {
        if fd >= 0 {
            _ = Darwin.close(fd)
            fd = -1
        }
    }

    // reconnectLoop retries connect() with exponential backoff (1s→2s→4s…32s cap)
    // until connected or cancelled. Use from a Task to get cooperative cancellation.
    public func reconnectLoop() async throws {
        var delay: UInt64 = 1_000_000_000 // 1s in ns
        let cap: UInt64 = 32_000_000_000  // 32s cap
        while true {
            try Task.checkCancellation()
            do {
                try connect()
                return
            } catch {
                try await Task.sleep(nanoseconds: delay)
                delay = min(delay * 2, cap)
            }
        }
    }
}
