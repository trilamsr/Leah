import AppKit

/// AppSuppression silences the wake-word when the frontmost app's bundle id
/// is on the meeting-apps list (§6.7). Bundle id (stable) — not process name.
public final class AppSuppression {
    public let bundles: Set<String>
    private let frontmostProvider: () -> String

    // Meeting-host apps that suppress wake-word capture. Browsers included
    // because Meet/Webex/Around/Pop are web-only. Chrome dropped from prior
    // list — false-positive on day-to-day browsing.
    public static let defaultMeetingBundles: Set<String> = [
        "us.zoom.xos",
        "com.microsoft.teams2",
        "com.apple.FaceTime",
        "com.tinyspeck.slackmacgap",
        "com.hnc.Discord",
        "Cisco-Systems.Spark", // Webex
        "com.apple.Safari",
        "org.mozilla.firefox",
        "com.brave.Browser",
        "com.google.Chrome", // kept — Meet via Chrome is the common path
    ]

    public init(bundles: Set<String> = AppSuppression.defaultMeetingBundles,
                stubFrontmost: String? = nil) {
        self.bundles = bundles
        if let s = stubFrontmost {
            self.frontmostProvider = { s }
        } else {
            self.frontmostProvider = {
                NSWorkspace.shared.frontmostApplication?.bundleIdentifier ?? ""
            }
        }
    }

    public func suppressedRightNow() -> Bool {
        bundles.contains(frontmostProvider())
    }
}
