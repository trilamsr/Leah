import SwiftUI

// Brand palette tokens shared by every tile view. Hex values are the source
// of truth in the design spec § 2 / § 2.6; defining them once here prevents
// cross-tile drift and the hidden-coupling trap of hosting the extension on
// one view file while other view files reference its symbols.
extension Color {
  // Champagne gold #C9A961 — brand-mark / accent token; never as background.
  public static let leahGold = Color(red: 201/255, green: 169/255, blue: 97/255)
  // Oxblood alert #D75A66 (AA-pass) — negative deltas, adverse-event markers.
  public static let leahRedAlert = Color(red: 215/255, green: 90/255, blue: 102/255)
  // Ivory #F2EDE0 — body text default; positive deltas; non-accent chart series @ 40%.
  public static let leahIvory = Color(red: 242/255, green: 237/255, blue: 224/255)
}
