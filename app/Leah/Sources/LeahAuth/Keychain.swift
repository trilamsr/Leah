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
    // Try update first; if no item exists, fall back to add. Atomic — a
    // kill between SecItemDelete + SecItemAdd previously wiped the prior
    // key without persisting the new one (round 9 keychain audit).
    let updateAttrs: [String: Any] = [
      kSecValueData as String: data,
      kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlocked,
    ]
    let updateStatus = SecItemUpdate(query as CFDictionary, updateAttrs as CFDictionary)
    if updateStatus == errSecSuccess { return }
    if updateStatus != errSecItemNotFound { throw KeychainError.status(updateStatus) }
    var insert = query
    insert[kSecValueData as String] = data
    insert[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlocked
    let addStatus = SecItemAdd(insert as CFDictionary, nil)
    guard addStatus == errSecSuccess else { throw KeychainError.status(addStatus) }
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
  // If save(newKey) fails after reading previous, restore previous before
  // re-throwing so failed rotation never wipes the prior key.
  public static func rotate(newKey: String, undoWindow: TimeInterval = 60) throws -> () -> Void {
    let previous = try read()
    let deadline = Date().addingTimeInterval(undoWindow)
    do {
      try save(newKey)
    } catch {
      // Restore prior key best-effort if save() partially completed.
      if let prev = previous {
        try? save(prev)
      }
      throw error
    }
    return {
      guard Date() < deadline, let prev = previous else { return }
      try? Keychain.save(prev)
    }
  }
}
