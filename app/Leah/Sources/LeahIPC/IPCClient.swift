import Foundation
import Darwin

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

    public enum IPCError: Error {
        case pathTooLong
        case writeFailed(errno: Int32)
        case readFailed(errno: Int32)
        case eof
    }

    public func connect() throws {
        let newFD = socket(AF_UNIX, SOCK_STREAM, 0)
        guard newFD >= 0 else { throw POSIXError(.EIO) }
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = Array(socketPath.utf8) + [0]
        guard pathBytes.count <= MemoryLayout.size(ofValue: addr.sun_path) else {
            _ = Darwin.close(newFD)
            throw IPCError.pathTooLong
        }
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
        try bytes.withUnsafeBytes { raw in
            guard let base = raw.baseAddress else { return }
            var sent = 0
            while sent < bytes.count {
                let n = Darwin.write(fd, base.advanced(by: sent), bytes.count - sent)
                if n < 0 {
                    if errno == EINTR { continue }
                    throw IPCError.writeFailed(errno: errno)
                }
                if n == 0 { throw IPCError.writeFailed(errno: EPIPE) }
                sent += n
            }
        }
    }

    public func receive() throws -> Frame {
        var hdr = [UInt8](repeating: 0, count: 4)
        try readFull(into: &hdr, count: 4)
        let n = UInt32(hdr[0]) << 24 | UInt32(hdr[1]) << 16 | UInt32(hdr[2]) << 8 | UInt32(hdr[3])
        guard Int(n) <= FrameCodec.maxFrameBytes else { throw FrameCodec.CodecError.oversize }
        var body = [UInt8](repeating: 0, count: Int(n))
        try readFull(into: &body, count: Int(n))
        return try JSONDecoder().decode(Frame.self, from: Data(body))
    }

    private func readFull(into buf: inout [UInt8], count: Int) throws {
        var got = 0
        while got < count {
            let n = buf.withUnsafeMutableBufferPointer { ptr -> Int in
                Darwin.read(fd, ptr.baseAddress!.advanced(by: got), count - got)
            }
            if n < 0 {
                if errno == EINTR { continue }
                throw IPCError.readFailed(errno: errno)
            }
            if n == 0 { throw IPCError.eof }
            got += n
        }
    }

    public func close() {
        if fd >= 0 {
            _ = Darwin.close(fd)
            fd = -1
        }
    }

    // 1s→32s exponential backoff; Task.checkCancellation gives cooperative cancel.
    public func reconnectLoop() async throws {
        var delay: UInt64 = 1_000_000_000
        let cap: UInt64 = 32_000_000_000
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
