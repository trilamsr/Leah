import SwiftUI
import AppKit

// "Obsidian Brass" code tile. Spec §10.1 item 9.
// Language is required (no guessing). Source capped at 16 KB.
// Gold (champagne) used ONLY for keywords and capped at 3 regions per visible
// tile to honor the canvas gold-budget invariant (§10.0).

public struct CodeTilePayload {
  public enum Error: Swift.Error, Equatable {
    case languageRequired
    case unsupportedLanguage(String)
    case sourceMissing
    case sourceTooLarge(Int)
  }

  public static let maxSourceBytes = 16 * 1024

  public let language: String
  public let filename: String?
  public let source: String

  public init(envelope: WidgetEnvelope) throws {
    let lang = (envelope.props["language"]?.value as? String) ?? ""
    if lang.isEmpty { throw Error.languageRequired }
    let norm = lang.lowercased()
    guard Self.supportedLanguages.contains(norm) else {
      throw Error.unsupportedLanguage(lang)
    }
    guard let src = envelope.props["source"]?.value as? String else {
      throw Error.sourceMissing
    }
    let byteLen = src.utf8.count
    if byteLen > Self.maxSourceBytes { throw Error.sourceTooLarge(byteLen) }
    self.language = norm
    self.filename = envelope.props["filename"]?.value as? String
    self.source = src
  }

  public struct Region: Equatable {
    public enum Role: Equatable { case keyword, identifier, op, whitespace }
    public let text: String
    public let role: Role
  }

  // Token scan. Gold (.keyword) is capped at goldBudget per visible tile so
  // the canvas-wide ≤3 invariant survives even when a panel pre-renders many
  // keywords above the fold.
  public static let goldBudget = 3

  public func highlightedRegions() -> [Region] {
    let kws = Self.keywords[language] ?? []
    var out: [Region] = []
    var goldUsed = 0
    var i = source.startIndex
    while i < source.endIndex {
      let ch = source[i]
      if ch.isWhitespace {
        let start = i
        while i < source.endIndex, source[i].isWhitespace { i = source.index(after: i) }
        out.append(Region(text: String(source[start..<i]), role: .whitespace))
      } else if ch.isLetter || ch == "_" {
        let start = i
        while i < source.endIndex, source[i].isLetter || source[i].isNumber || source[i] == "_" {
          i = source.index(after: i)
        }
        let word = String(source[start..<i])
        if kws.contains(word) && goldUsed < Self.goldBudget {
          goldUsed += 1
          out.append(Region(text: word, role: .keyword))
        } else {
          out.append(Region(text: word, role: .identifier))
        }
      } else {
        let start = i
        i = source.index(after: i)
        out.append(Region(text: String(source[start..<i]), role: .op))
      }
    }
    return out
  }

  static let supportedLanguages: Set<String> = [
    "go", "swift", "python", "py", "javascript", "js", "typescript", "ts",
    "rust", "rs", "c", "cpp", "c++", "java", "kotlin", "ruby", "rb",
    "sql", "shell", "sh", "bash", "json", "yaml", "yml", "html", "css",
  ]

  static let keywords: [String: Set<String>] = [
    "go":         ["func", "var", "const", "package", "import", "return", "if", "else", "for", "range", "type", "struct", "interface", "select", "case", "default", "switch", "go", "defer", "chan", "map"],
    "swift":      ["func", "var", "let", "return", "if", "else", "for", "in", "while", "guard", "switch", "case", "default", "struct", "class", "enum", "protocol", "import", "throw", "throws", "try", "catch", "do"],
    "python":     ["def", "class", "import", "from", "return", "if", "elif", "else", "for", "while", "with", "as", "try", "except", "finally", "raise", "pass", "lambda", "yield", "True", "False", "None"],
    "py":         ["def", "class", "import", "from", "return", "if", "elif", "else", "for", "while", "with", "as", "try", "except", "finally", "raise", "pass", "lambda", "yield"],
    "javascript": ["function", "var", "let", "const", "return", "if", "else", "for", "while", "switch", "case", "default", "class", "import", "from", "export", "throw", "try", "catch", "finally", "new"],
    "js":         ["function", "var", "let", "const", "return", "if", "else", "for", "while", "class", "import", "from", "export"],
    "typescript": ["function", "var", "let", "const", "return", "if", "else", "for", "while", "class", "interface", "type", "import", "from", "export", "enum"],
    "ts":         ["function", "var", "let", "const", "return", "if", "else", "class", "interface", "type", "import", "from", "export", "enum"],
    "rust":       ["fn", "let", "mut", "pub", "use", "mod", "struct", "enum", "impl", "trait", "for", "while", "loop", "if", "else", "match", "return", "self", "Self"],
    "rs":         ["fn", "let", "mut", "pub", "use", "mod", "struct", "enum", "impl", "trait", "for", "while", "loop", "if", "else", "match", "return"],
    "c":          ["int", "char", "void", "if", "else", "for", "while", "return", "struct", "typedef", "static", "const"],
    "cpp":        ["int", "char", "void", "if", "else", "for", "while", "return", "struct", "class", "public", "private", "protected", "template", "typename", "const", "static"],
    "c++":        ["int", "char", "void", "if", "else", "for", "while", "return", "struct", "class", "public", "private", "protected", "template", "typename"],
    "java":       ["public", "private", "protected", "class", "interface", "extends", "implements", "return", "if", "else", "for", "while", "switch", "case", "default", "static", "final", "void", "int", "boolean", "new", "throw", "throws"],
    "kotlin":     ["fun", "val", "var", "class", "interface", "object", "return", "if", "else", "for", "while", "when", "import", "package"],
    "ruby":       ["def", "end", "class", "module", "if", "elsif", "else", "unless", "while", "until", "return", "do", "begin", "rescue"],
    "rb":         ["def", "end", "class", "module", "if", "elsif", "else", "unless", "while", "return", "do", "begin", "rescue"],
    "sql":        ["select", "from", "where", "join", "left", "right", "inner", "outer", "on", "group", "by", "order", "having", "insert", "update", "delete", "into", "values", "as"],
    "shell":      ["if", "then", "else", "elif", "fi", "for", "in", "do", "done", "while", "case", "esac", "function", "return"],
    "sh":         ["if", "then", "else", "elif", "fi", "for", "in", "do", "done", "while", "case", "esac"],
    "bash":       ["if", "then", "else", "elif", "fi", "for", "in", "do", "done", "while", "case", "esac", "function"],
    "json":       [],
    "yaml":       [],
    "yml":        [],
    "html":       [],
    "css":        [],
  ]
}

public struct CodeTileView: View {
  let payload: CodeTilePayload

  public init(payload: CodeTilePayload) { self.payload = payload }

  public var body: some View {
    VStack(alignment: .leading, spacing: 0) {
      header
      Rectangle().fill(CodeDiffPalette.hairline).frame(height: 1)
      ScrollView(.vertical) { codeBody.padding(16) }
    }
    .background(CodeDiffPalette.obsidian1)
    .cornerRadius(12)
    .overlay(
      RoundedRectangle(cornerRadius: 12).strokeBorder(CodeDiffPalette.hairline, lineWidth: 1)
    )
  }

  private var header: some View {
    HStack {
      Text(payload.language.uppercased())
        .font(.caption.weight(.medium))
        .tracking(0.5)
        .foregroundColor(CodeDiffPalette.textMuted)
      if let f = payload.filename {
        Text("· \(f)")
          .font(.caption)
          .foregroundColor(CodeDiffPalette.textMuted)
      }
      Spacer()
      Button(action: copyToPasteboard) {
        Text("Copy").font(.caption.weight(.medium)).tracking(0.5)
      }
      .buttonStyle(.borderless)
      .foregroundColor(CodeDiffPalette.textMuted)
      .accessibilityLabel("Copy code")
    }
    .padding(.horizontal, 16).padding(.vertical, 10)
  }

  private var codeBody: some View {
    let regions = payload.highlightedRegions()
    return regions.reduce(Text("")) { acc, r in
      acc + Text(r.text)
        .font(.system(.callout, design: .monospaced))
        .foregroundColor(color(for: r.role))
    }
    .frame(maxWidth: .infinity, alignment: .leading)
  }

  private func color(for role: CodeTilePayload.Region.Role) -> Color {
    switch role {
    case .keyword:    return CodeDiffPalette.champagneGold
    case .identifier: return CodeDiffPalette.ivory
    case .op:         return CodeDiffPalette.textMuted
    case .whitespace: return CodeDiffPalette.ivory
    }
  }

  private func copyToPasteboard() {
    let pb = NSPasteboard.general
    pb.clearContents()
    pb.setString(payload.source, forType: .string)
  }
}

// Chrome tokens internal to code + diff tiles. Brand accents come from
// `Color.leah*` in LeahPalette.swift; obsidian/hairline/text-muted stay
// local because no other tile needs the body-bg + 20% hairline pairing.
enum CodeDiffPalette {
  static let obsidian1     = Color(red: 0x0E/255, green: 0x10/255, blue: 0x14/255)
  static let champagneGold = Color.leahGold
  static let redAlert      = Color.leahRedAlert
  static let ivory         = Color.leahIvory
  static let textMuted     = Color(red: 0xB8/255, green: 0xB0/255, blue: 0xA0/255)
  static let hairline      = Color.white.opacity(0.20)
}
