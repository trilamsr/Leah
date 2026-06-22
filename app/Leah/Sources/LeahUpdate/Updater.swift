import Foundation
import Sparkle

public final class Updater {
    public let feedURL: String
    private let controller: SPUStandardUpdaterController

    public init(feedURL: String) {
        self.feedURL = feedURL
        self.controller = SPUStandardUpdaterController(
            startingUpdater: true,
            updaterDelegate: nil,
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

    public func checkForUpdates() {
        controller.checkForUpdates(nil)
    }

    public var canCheckForUpdates: Bool {
        controller.updater.canCheckForUpdates
    }
}
