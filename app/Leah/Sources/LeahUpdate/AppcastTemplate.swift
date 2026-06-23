import Foundation

// Generates the XML <item> element for a new release entry in appcast.xml.
public enum AppcastTemplate {
    public struct Item {
        public let version: String
        public let downloadURL: String
        public let length: Int
        public let eddsaSignature: String
        public let pubDate: Date
        public let channel: String

        public init(
            version: String,
            downloadURL: String,
            length: Int,
            eddsaSignature: String,
            pubDate: Date = Date(),
            channel: String = "stable"
        ) {
            self.version = version
            self.downloadURL = downloadURL
            self.length = length
            self.eddsaSignature = eddsaSignature
            self.pubDate = pubDate
            self.channel = channel
        }
    }

    private static let rfc822: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "EEE, dd MMM yyyy HH:mm:ss Z"
        f.timeZone = TimeZone(identifier: "UTC")
        return f
    }()

    // XML attribute escape — URLs with & and signatures with quotes would
    // otherwise break the appcast parse.
    private static func xmlAttr(_ s: String) -> String {
        s.replacingOccurrences(of: "&", with: "&amp;")
            .replacingOccurrences(of: "<", with: "&lt;")
            .replacingOccurrences(of: "\"", with: "&quot;")
    }

    public static func item(_ i: Item) -> String {
        let url = xmlAttr(i.downloadURL)
        let ver = xmlAttr(i.version)
        let sig = xmlAttr(i.eddsaSignature)
        let ch = xmlAttr(i.channel)
        return """
        <item>
          <title>Leah \(ver)</title>
          <pubDate>\(rfc822.string(from: i.pubDate))</pubDate>
          <sparkle:channel>\(ch)</sparkle:channel>
          <enclosure
            url="\(url)"
            sparkle:version="\(ver)"
            length="\(i.length)"
            type="application/octet-stream"
            sparkle:edSignature="\(sig)" />
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
