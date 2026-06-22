## Task 12 Report — Widget Primitives (Stat, Table, List)

**Status:** DONE

**Commit:** d854766

**Branch:** worktree-agent-af0641b5532ea96fc

---

### Files created

| File | Description |
|---|---|
| `app/Leah/Sources/LeahWidgets/Widget.swift` | `LeahWidget` protocol + `WidgetSize` enum |
| `app/Leah/Sources/LeahWidgets/Tokens.swift` | `Palette` namespace — §2.6 hex constants as `Color` |
| `app/Leah/Sources/LeahWidgets/Stat.swift` | `StatWidget: LeahWidget` — KPI tile, gold/+delta, oxblood/-delta |
| `app/Leah/Sources/LeahWidgets/Table.swift` | `TableWidget: LeahWidget` — sortable rows, gold header, oxblood negative cells |
| `app/Leah/Sources/LeahWidgets/List.swift` | `ListWidget: LeahWidget` — ordered/unordered, scrollable |
| `app/Leah/Sources/LeahWidgets/WidgetEnvelope.swift` | `WidgetEnvelope: Codable` + `AnyCodable` heterogeneous JSON |
| `app/Leah/Tests/LeahWidgetsTests/WidgetTests.swift` | 14 value-driven tests (RED first, no snapshot deps) |

### Package.swift changes

Added `LeahWidgets` library target and `LeahWidgetsTests` test target.

---

### TDD sequence

**RED output (before implementation):**
```
error: no such module 'LeahWidgets'
```

**GREEN output:**
```
Executed 14 tests, with 0 failures (0 unexpected) in 0.014 seconds
```

### Test command
```bash
cd app/Leah && swift test --filter LeahWidgetsTests
```

---

### Self-review

- Protocol: `LeahWidget: View` with `size: WidgetSize` — conforms to task prompt interface spec.
- Palette: all §2.6 hex values exact (`#08090C`, `#0E1014`, `#161922`, `#22262F`, `#C9A961`, `#7A1F2B`, `#F2EDE0`).
- Hairline: 1px @ 20% opacity via `Color.white.opacity(0.20)` + `strokeBorder`.
- Corner radius: 12px per spec §19.
- `StatWidget.trend`: gold for `+` prefix, oxblood for `-` prefix, absent when nil.
- `TableWidget.negativeMask`: oxblood on `true` cells, ivory on `false`/absent.
- `ListWidget.ordered`: numbered prefix vs gold bullet.
- `AnyCodable`: covers String/Int/Double/Bool/Array/Object encode+decode.
- No AI signatures. No ceremony.

### Concerns

1. **FocusPanelView.swift not modified.** `LeahUI` target does not exist yet (Task 11 pending merge). FocusPanelView wiring deferred to Task 11.
2. **`render()` free function omitted.** Not listed in task prompt interface contract; requires SwiftUI import in envelope file. Can be added in Task 11 alongside FocusPanelView.
3. **Tests are value-driven, not snapshot.** SwiftUI snapshot tests require ViewInspector or Xcode UI test runner — both add unowned dependencies. Value tests on `.body`, JSON decode, Palette, and protocol conformance cover the contract without extra deps.
