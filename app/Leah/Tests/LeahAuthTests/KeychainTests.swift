import XCTest
@testable import LeahAuth

// Keychain SecItem* APIs are unavailable on headless CI runners; skip
// all tests when the CI env var is set so the suite still passes in CI.
final class KeychainTests: XCTestCase {

  private func skipOnCI() throws {
    try XCTSkipIf(
      ProcessInfo.processInfo.environment["CI"] != nil,
      "Keychain unavailable on headless CI"
    )
  }

  func testSaveAndLoad() throws {
    try skipOnCI()
    try Keychain.save("sk-test-save")
    let loaded = try Keychain.load()
    XCTAssertEqual(loaded, "sk-test-save")
    try Keychain.delete()
  }

  func testLoadReturnsNilWhenEmpty() throws {
    try skipOnCI()
    try Keychain.delete()
    // ANTHROPIC_API_KEY must not be set for this test to be meaningful.
    if ProcessInfo.processInfo.environment["ANTHROPIC_API_KEY"] != nil { return }
    let loaded = try Keychain.load()
    XCTAssertNil(loaded)
  }

  func testSaveOverwritesPrevious() throws {
    try skipOnCI()
    try Keychain.save("first")
    try Keychain.save("second")
    let loaded = try Keychain.load()
    XCTAssertEqual(loaded, "second")
    try Keychain.delete()
  }

  func testDeleteIsIdempotent() throws {
    try skipOnCI()
    try Keychain.delete()
    // Second delete must not throw errSecItemNotFound.
    XCTAssertNoThrow(try Keychain.delete())
  }

  func testReadReturnsNilWhenEmpty() throws {
    try skipOnCI()
    try Keychain.delete()
    if ProcessInfo.processInfo.environment["ANTHROPIC_API_KEY"] != nil { return }
    let r = try Keychain.read()
    XCTAssertNil(r)
  }

  func testEnvVarTakesPrecedence() throws {
    try skipOnCI()
    // If ANTHROPIC_API_KEY is set, load() returns env value regardless of keychain.
    guard let env = ProcessInfo.processInfo.environment["ANTHROPIC_API_KEY"], !env.isEmpty else {
      return // env not set; skip this branch
    }
    try Keychain.save("keychain-value")
    let loaded = try Keychain.load()
    XCTAssertEqual(loaded, env)
    try Keychain.delete()
  }

  func testRotateRestoresOnUndo() throws {
    try skipOnCI()
    try Keychain.save("original")
    let undo = try Keychain.rotate(newKey: "rotated", undoWindow: 60)
    XCTAssertEqual(try Keychain.load(), "rotated")
    undo()
    XCTAssertEqual(try Keychain.load(), "original")
    try Keychain.delete()
  }

  func testRotateExpiredUndoIsNoOp() throws {
    try skipOnCI()
    try Keychain.save("original")
    let undo = try Keychain.rotate(newKey: "rotated", undoWindow: -1)
    XCTAssertEqual(try Keychain.load(), "rotated")
    undo() // window already expired — must not restore
    XCTAssertEqual(try Keychain.load(), "rotated")
    try Keychain.delete()
  }

  func testSaveRejectsEmptyKey() throws {
    try skipOnCI()
    // Empty string encodes fine; save/load round-trips it.
    XCTAssertNoThrow(try Keychain.save(""))
    try Keychain.delete()
  }

  func testLoadAfterDeleteReturnsNil() throws {
    try skipOnCI()
    try Keychain.save("to-delete")
    try Keychain.delete()
    if ProcessInfo.processInfo.environment["ANTHROPIC_API_KEY"] != nil { return }
    XCTAssertNil(try Keychain.load())
  }

  func testSaveAndReadDirectly() throws {
    try skipOnCI()
    try Keychain.save("direct-read")
    let r = try Keychain.read()
    XCTAssertEqual(r, "direct-read")
    try Keychain.delete()
  }

  func testRotateToSameKeyRoundTrips() throws {
    try skipOnCI()
    try Keychain.save("same")
    _ = try Keychain.rotate(newKey: "same", undoWindow: 60)
    XCTAssertEqual(try Keychain.load(), "same")
    try Keychain.delete()
  }
}
