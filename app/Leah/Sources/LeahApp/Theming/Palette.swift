import SwiftUI
import AppKit

public struct Palette: Equatable, Sendable {
  public enum Mode: String, Sendable, CaseIterable { case dark, light }

  public let mode: Mode
  public let obsidian1: Color
  public let obsidian2: Color
  public let obsidian3: Color
  public let ivoryText: Color
  public let textMuted: Color
  public let goldPrimary: Color
  public let goldPrimaryLight: Color
  public let goldMutedLight: Color
  public let redAlert: Color

  public let allHex: [String]

  public static func forMode(_ mode: Mode) -> Palette {
    switch mode {
    case .dark:  return darkPalette
    case .light: return lightPalette
    }
  }

  // Spec §3.1 dark tokens — obsidian-1/2/3, ivory text, muted text, gold-primary, red-alert.
  // Light gold-primary + gold-muted exposed as -Light variants for cross-mode pairing
  // (mark/pulse needs both palettes resolvable per spec §3.6 cross-fade rule).
  private static let darkObsidian1Hex = "#0E1014"
  private static let darkObsidian2Hex = "#161922"
  private static let darkObsidian3Hex = "#22262F"
  private static let darkIvoryHex     = "#F2EDE0"
  private static let darkMutedHex     = "#B8B0A0"
  private static let darkGoldHex      = "#C9A961"
  private static let darkRedAlertHex  = "#D75A66"
  private static let lightGoldHex     = "#7A6332"
  private static let lightGoldMutedHex = "#A88F58"

  private static let lightBone1Hex    = "#F2EFE8"
  private static let lightBone2Hex    = "#EAE6DD"
  private static let lightBone3Hex    = "#DCD7CC"
  private static let lightTextPrimary = "#1E1D1A"
  private static let lightTextMuted   = "#3D3A33"
  private static let lightRedAlertHex = "#A8323D"

  private static let darkPalette = Palette(
    mode: .dark,
    obsidian1: hex(darkObsidian1Hex),
    obsidian2: hex(darkObsidian2Hex),
    obsidian3: hex(darkObsidian3Hex),
    ivoryText: hex(darkIvoryHex),
    textMuted: hex(darkMutedHex),
    goldPrimary: hex(darkGoldHex),
    goldPrimaryLight: hex(lightGoldHex),
    goldMutedLight: hex(lightGoldMutedHex),
    redAlert: hex(darkRedAlertHex),
    allHex: [
      darkObsidian1Hex, darkObsidian2Hex, darkObsidian3Hex,
      darkIvoryHex, darkMutedHex, darkGoldHex, darkRedAlertHex,
      lightGoldHex, lightGoldMutedHex,
    ]
  )

  private static let lightPalette = Palette(
    mode: .light,
    obsidian1: hex(lightBone1Hex),
    obsidian2: hex(lightBone2Hex),
    obsidian3: hex(lightBone3Hex),
    ivoryText: hex(lightTextPrimary),
    textMuted: hex(lightTextMuted),
    goldPrimary: hex(lightGoldHex),
    goldPrimaryLight: hex(lightGoldHex),
    goldMutedLight: hex(lightGoldMutedHex),
    redAlert: hex(lightRedAlertHex),
    allHex: [
      lightBone1Hex, lightBone2Hex, lightBone3Hex,
      lightTextPrimary, lightTextMuted,
      lightGoldHex, lightGoldMutedHex, lightRedAlertHex,
    ]
  )

  private static func hex(_ s: String) -> Color {
    var t = s
    if t.hasPrefix("#") { t.removeFirst() }
    let v = UInt32(t, radix: 16) ?? 0
    let r = Double((v >> 16) & 0xFF) / 255.0
    let g = Double((v >> 8) & 0xFF) / 255.0
    let b = Double(v & 0xFF) / 255.0
    return Color(red: r, green: g, blue: b)
  }
}
