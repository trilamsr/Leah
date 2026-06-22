import XCTest
@testable import LeahUI

final class VoicePaneTests: XCTestCase {
    override func setUp() {
        super.setUp()
        UserDefaults.standard.removeObject(forKey: VoicePane.ttsProviderKey)
        UserDefaults.standard.removeObject(forKey: VoicePane.wakeWordKey)
    }

    func testVoicePaneTTSSelectorDisabledWithPhase3Tooltip() {
        XCTAssertFalse(VoicePane.ttsRuntimeEnabled,
                       "TTS provider selector must be disabled until Phase 3 runtime lands")
        XCTAssertEqual(VoicePane.ttsPhase3Tooltip, "Phase 3 lands runtime")
        XCTAssertEqual(VoicePane.ttsProviderOptions, ["ElevenLabs", "Apple Ava"])
        VoicePane.setTTSProvider("Apple Ava")
        XCTAssertEqual(VoicePane.currentTTSProvider(), "Apple Ava",
                       "Selection must persist even though runtime is disabled")
    }

    func testWakeWordTogglePersists() {
        XCTAssertFalse(VoicePane.wakeWordEnabled(),
                       "Wake-word default must be OFF")
        VoicePane.setWakeWordEnabled(true)
        XCTAssertTrue(VoicePane.wakeWordEnabled())
        VoicePane.setWakeWordEnabled(false)
        XCTAssertFalse(VoicePane.wakeWordEnabled())
    }
}
