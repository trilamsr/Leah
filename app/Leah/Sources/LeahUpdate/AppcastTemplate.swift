import Foundation

// Generates the XML <item> element for a new release entry in appcast.xml.
public enum AppcastTemplate {
    public struct Item {
        public let version: String
        public let downloadURL: String
        public let length: Int
        public let eddsaSignature: String
        public let pubDate: Date

        public init(
            version: String,
            downloadURL: String,
            length: Int,
            eddsaSignature: String,
            pubDate: Date = Date()
        ) {
            self.version = version
            self.downloadURL = downloadURL
            self.length = length
            self.eddsaSignature = eddsaSignature
            self.pubDate = pubDate
        }
    }

    private static let rfc822: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "EEE, dd MMM yyyy HH:mm:ss Z"
        f.timeZone = TimeZone(identifier: "UTC")
        return f
    }()

    public static func item(_ i: Item) -> String {
        """
        <item>
          <title>Leah \(i.version)</title>
          <pubDate>\(rfc822.string(from: i.pubDate))</pubDate>
          <enclosure
            url="\(i.downloadURL)"
            sparkle:version="\(i.version)"
            length="\(i.length)"
            type="application/octet-stream"
            sparkle:edSignature="\(i.eddsaSignature)" />
        </item>
        """
    }

    public static let feedSeed = """
        <?xml version="1.0" encoding="utf-8"?>
        <rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
          <channel>
            <title>Leah</title>
            <link>https://maydow.github.io/leah/appcast.xml</link>
            <description>Leah update feed</description>
            <language>en</language>
          </channel>
        </rss>
        """
}
