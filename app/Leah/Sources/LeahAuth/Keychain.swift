import Foundation
import Security

public enum KeychainError: Error {
  case encodingFailed
  case status(OSStatus)
}

public enum Keychain {
  public static let service = "com.maydow.leah.anthropic"
  public static let account = "default"

  // ANTHROPIC_API_KEY env takes precedence over stored key
  public static func load() throws -> String? {
    if let env = ProcessInfo.processInfo.environment["ANTHROPIC_API_KEY"], !env.isEmpty {
      return env
    }
    return try read()
  }

  public static func save(_ key: String) throws {
    guard let data = key.data(using: .utf8) else { throw KeychainError.encodingFailed }
    let query: [String: Any] = [
      kSecClass as String: kSecClassGenericPassword,
      kSecAttrService as String: service,
      kSecAttrAccount as String: account,
    ]
    SecItemDelete(query as CFDictionary)
    var insert = query
    insert[kSecValueData as String] = data
    insert[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlocked
    let s = SecItemAdd(insert as CFDictionary, nil)
    guard s == errSecSuccess else { throw KeychainError.status(s) }
  }

  public static func read() throws -> String? {
    let query: [String: Any] = [
      kSecClass as String: kSecClassGenericPassword,
      kSecAttrService as String: service,
      kSecAttrAccount as String: account,
      kSecReturnData as String: true,
      kSecMatchLimit as String: kSecMatchLimitOne,
    ]
    var out: AnyObject?
    let s = SecItemCopyMatching(query as CFDictionary, &out)
    if s == errSecItemNotFound { return nil }
    guard s == errSecSuccess, let data = out as? Data else { throw KeychainError.status(s) }
    guard let key = String(data: data, encoding: .utf8) else { throw KeychainError.encodingFailed }
    return key
  }

  public static func delete() throws {
    let query: [String: Any] = [
      kSecClass as String: kSecClassGenericPassword,
      kSecAttrService as String: service,
      kSecAttrAccount as String: account,
    ]
    let s = SecItemDelete(query as CFDictionary)
    guard s == errSecSuccess || s == errSecItemNotFound else { throw KeychainError.status(s) }
  }

  // Returns an undo closure valid for undoWindow seconds. Calling it after the
  // window restores nothing (the previous key is no longer recoverable).
  public static func rotate(newKey: String, undoWindow: TimeInterval = 60) throws -> () -> Void {
    let previous = try read()
    let deadline = Date().addingTimeInterval(undoWindow)
    try save(newKey)
    return {
      guard Date() < deadline, let prev = previous else { return }
      try? Keychain.save(prev)
    }
  }
}
