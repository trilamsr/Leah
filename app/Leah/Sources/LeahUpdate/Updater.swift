import Foundation
import Sparkle

// Delegate carries the channel allow-list into SPUUpdater so that flipping
// "Use rollback channel" in Settings → Advanced widens the filter to include
// items tagged <sparkle:channel>rollback</sparkle:channel>.
public final class ChannelDelegate: NSObject, SPUUpdaterDelegate {
    public var policy: UpdateChannelPolicy
    public init(policy: UpdateChannelPolicy) { self.policy = policy }
    public func allowedChannels(for updater: SPUUpdater) -> Set<String> {
        Set(policy.allowedChannels)
    }
}

public final class Updater {
    public let feedURL: String
    private let controller: SPUStandardUpdaterController
    private let delegate: ChannelDelegate

    public init(feedURL: String, channelPolicy: UpdateChannelPolicy = UpdateChannelPolicy(useRollback: false)) {
        self.feedURL = feedURL
        let d = ChannelDelegate(policy: channelPolicy)
        self.delegate = d
        self.controller = SPUStandardUpdaterController(
            startingUpdater: true,
            updaterDelegate: d,
            userDriverDelegate: nil
        )
        if let url = URL(string: feedURL) {
            self.controller.updater.setFeedURL(url)
        } else {
            // Silent failure would leave updater pointed at no feed; surface it.
            NSLog("LeahUpdate: invalid feedURL %@ — updates disabled", feedURL)
        }
        self.controller.updater.automaticallyChecksForUpdates = true
        self.controller.updater.updateCheckInterval = 86_400
    }

    public func setChannelPolicy(_ p: UpdateChannelPolicy) {
        delegate.policy = p
    }

    public func checkForUpdates() {
        controller.checkForUpdates(nil)
    }

    public var canCheckForUpdates: Bool {
        controller.updater.canCheckForUpdates
    }
}
