import SwiftUI

public enum Palette {
    // §2.6 obsidian backgrounds
    public static let obsidian0 = Color(red: 0x08/255, green: 0x09/255, blue: 0x0C/255) // #08090C
    public static let obsidian1 = Color(red: 0x0E/255, green: 0x10/255, blue: 0x14/255) // #0E1014
    public static let obsidian2 = Color(red: 0x16/255, green: 0x19/255, blue: 0x22/255) // #161922
    public static let obsidian3 = Color(red: 0x22/255, green: 0x26/255, blue: 0x2F/255) // #22262F

    // §2.6 accent
    public static let champagneGold = Color(red: 0xC9/255, green: 0xA9/255, blue: 0x61/255) // #C9A961
    public static let oxblood      = Color(red: 0x7A/255, green: 0x1F/255, blue: 0x2B/255) // #7A1F2B
    public static let ivory        = Color(red: 0xF2/255, green: 0xED/255, blue: 0xE0/255) // #F2EDE0

    public static let textMuted    = Color(red: 0xB8/255, green: 0xB0/255, blue: 0xA0/255)
    public static let hairline     = Color.white.opacity(0.20)
}

extension Palette {
    // mirrors AppearancePane.minimalModeEnabled() — duplicated key avoids
    // a LeahWidgets→LeahUI dependency for a one-line UserDefaults read.
    public static var minimalMode: Bool {
        UserDefaults.standard.bool(forKey: "leah.appearance.minimalMode")
    }

    // tiles call accentGold instead of champagneGold so the toggle has
    // render-time effect; clear collapses the ornamental gold accent.
    public static var accentGold: Color {
        minimalMode ? .clear : champagneGold
    }
}
