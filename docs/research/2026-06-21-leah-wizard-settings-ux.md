# Leah — First-launch wizard + Settings UX

> Author: senior UX designer (Linear onboarding / Notion first-run / Things permissions / Arc setup / Cron-Notion-Calendar lineage)
> Date: 2026-06-21
> Status: companion spec to `2026-06-21-leah-macos-native-ui-design-v1.md`. Honors that doc's palette, sigil, motion, NSPanel chamber model, and accessibility rules.
> Scope: two surfaces only — (A) first-launch wizard, (B) Settings preferences pane.

---

## 0. Operator decisions encoded here (deltas from the v1 design doc)

| Decision | v1 design doc | This spec |
|---|---|---|
| Global hotkey | `⌘⇧Space` | **`⌘⌃` (Cmd+Ctrl) — modifier-only chord**, per operator. Confirmable + customizable in wizard step 2. |
| Wake-word default | OFF | **ON** — operator explicitly flipped it. Wizard step 4 still presents the toggle (transparent), but pre-checked. |
| Permissions strategy | Wizard asks for mic + accessibility + screen-recording up front | **Permissions accumulate over time.** Wizard asks ONLY mic (load-bearing for step 4 wake-word demo). Accessibility, screen-recording, automation, contacts, reminders, focus-mode, notifications deferred to Settings → Permissions (lazy-prompted on first use). |
| Integrations in wizard | 6 services (Calendar / Mail / GitHub / Linear / arxiv / News) | **ONE primary** (operator picks Calendar **or** Mail **or** Files at step 5). The rest live in Settings → Integrations. Rationale: prove value with one connection; defer the rest. |
| Wizard length | 6 steps | **5 steps** (welcome / hotkey / mic / one-integration / ready). Wake-word toggle folds into the mic step, not its own screen — it is a property of "how you talk to Leah". HUD-position picker deferred to Settings → General (operator drags HUD; setting reflects). |

These are the deviations. Everything else (palette, sigil, motion tokens, NSPanel chamber, hairline rules, type stack, reduced-motion behavior) is inherited verbatim from the v1 design doc and cited inline below where relevant.

---

# Part A — First-launch wizard

## A.1 Design principles applied

| Principle | Source | How it shows up here |
|---|---|---|
| Progressive disclosure | Nielsen #8, Apple HIG | 5 steps, 1 job each. 13+ optional permissions deferred to Settings → Permissions (lazy-prompt on first use). |
| One job per screen | Linear onboarding, Things first-run | Each screen has exactly one decision + one primary CTA. The wake-word toggle is co-located on the mic screen because consenting to wake-word IS consenting to the mic — same physical permission, same mental model. |
| Skippable | Notion first-run, Cron setup | Steps 2 (hotkey), 4 (integration) are skippable. Steps 1 (welcome), 3 (mic), 5 (ready) are not skippable but are zero-cost (welcome = "Begin"; ready = "Done"). Mic itself is not skippable as a *step* but the *grant* is skippable — operator can say "Not now" and still finish; wake-word silently disables. |
| Show value before asking | Arc setup, Things permissions | Mic screen demos wake-word with a live waveform visualizer BEFORE the OS permission prompt. Operator hears Leah say "Hi, I'm Leah" via TTS (which needs no permission) before granting mic. Hotkey screen flashes a mock focus-chamber summon when the chord is pressed during the recorder. |
| Transparency | Apple HIG, GDPR norms | Every permission line: WHY in 1 sentence + concrete example + which OS pane it opens. No "we need this for the best experience" copy — that's the dark-pattern smell. |
| Reversible | Universal | Every step has Back (except step 1). Quit during wizard → app remains installed; re-shows wizard next launch. No partial-state lock. |
| Defer-friendly | Notion, Cron, Granola | Each integration row reads "Set up now (≈30s) — or later from Settings". Skip is equally weighted with Connect (same button size, not a dim text-link). |
| No dead-ends | Nielsen #9 | Every error has a path forward (see A.4 error matrix). Permission denied → "Open System Settings" + "Skip for now"; both work. |
| Native chrome | Apple HIG | NSWindow, hidden titlebar, `.titled` style with `.fullSizeContentView` and traffic lights moved to top-left at 20pt inset (per v1 chamber spec). Resizable: NO — wizard is fixed 720×520 (matches v1 §1.5). |
| Anti-pattern avoidance | Per §A.5 | No fake loading. No "Are you sure you want to skip?" guilt. No countdown timers. No pre-checked email opt-in. |

## A.2 Wizard flow diagram

```
                            ┌─────────────────┐
                            │  Launch Leah    │
                            └────────┬────────┘
                                     │ first launch only
                                     ▼
                            ┌─────────────────┐
                            │ 1. Welcome      │  no skip; CTA "Begin"
                            │   sigil hero    │
                            └────────┬────────┘
                                     │
                                     ▼
                            ┌─────────────────┐
                            │ 2. Hotkey       │  default ⌘⌃; "Skip" → keep default
                            │   confirm/      │  click-record → re-listen → conflict check
                            │   re-record     │  CTA "Continue"
                            └────────┬────────┘
                                     │
                                     ▼
                            ┌─────────────────┐
                            │ 3. Voice +      │  TTS demo plays "Hi, I'm Leah" (no perm)
                            │    mic +        │  ↓
                            │    wake-word    │  CTA "Enable voice" → OS mic prompt
                            │                 │    granted → wake-word demo waveform
                            │                 │    denied  → "Continue without voice"
                            │                 │  Toggle: "Listen for 'Leah'" ON (pre-checked)
                            │                 │  "Skip" → continue text-only
                            └────────┬────────┘
                                     │
                                     ▼
                            ┌─────────────────┐
                            │ 4. One thing    │  3 radio cards: Calendar / Mail / Files
                            │    to connect   │  Pick one → OAuth/system-picker handoff
                            │                 │  "Skip — set up later" equally weighted
                            └────────┬────────┘
                                     │
                                     ▼
                            ┌─────────────────┐
                            │ 5. You're ready │  Big hotkey reminder; "Open Settings"
                            │                 │  CTA "Done" (closes wizard, ambient HUD appears)
                            └─────────────────┘
                                     │
                                     ▼
                            ┌─────────────────┐
                            │ Ambient HUD     │  bottom-right; first toast: "Press ⌘⌃ to ask"
                            │   appears       │  (6s, dismissable)
                            └─────────────────┘
```

**Re-entry**: Settings → "Re-run setup" returns to step 1. Existing grants are not revoked; the wizard reflects current state (e.g., mic shows "Already granted ✓").

**Abort**: Esc on any step → confirm sheet "Quit setup? You can finish anytime from the menubar." [Quit] [Keep setting up]. Quit closes the window; menubar item shows a `⬡` with a small gold dot meaning "setup pending"; clicking it re-opens wizard at last step.

## A.3 Per-screen wireframes

### Step 1 — Welcome

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ○  ○  ○  ○                                          step 1/5│
│  ──────────────────────────────────────────────────────────────  │
│                                                                  │
│                                                                  │
│                                                                  │
│                                                                  │
│                              ⬡                                   │
│                          (96px sigil)                            │
│                                                                  │
│                          Hi, I'm Leah.                           │
│                  Your personal assistant.                        │
│                                                                  │
│                                                                  │
│                                                                  │
│                        [   Begin   ]                            │
│                                                                  │
│                                                                  │
│                   Takes about a minute.                          │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

- **Copy**: "Hi, I'm Leah." (Tiempos italic, 36pt, ivory) / "Your personal assistant." (Inter 400, 18pt, `--text-muted`).
- **Sigil**: 96px, full Flourish-2 sigil-acknowledgment animation on entry (one rotation tick + warm pulse). Per v1 §3.4.
- **Mic-line preview**: as soon as the sigil settles (380ms), TTS plays "Hi, I'm Leah." at a low volume using the system default voice — needs no permission, sets the auditory brand. Operator can mute the wizard from a tiny `🔇` glyph top-right (mutes wizard only, not the app).
- **Button**: gold-primary fill, ivory text, 160×40pt, single CTA, centered.
- **Footnote**: "Takes about a minute." (`--text-dim`, 12pt). Sets expectations — Nielsen calibration heuristic.
- **No skip.** "Begin" is the path. Esc opens the abort sheet.
- **Reduced motion**: sigil cross-fades in; no rotation tick.

### Step 2 — Hotkey

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ●  ○  ○  ○                                          step 2/5│
│  ──────────────────────────────────────────────────────────────  │
│                                                                  │
│   How will you call Leah?                                        │
│   Press a hotkey from any app to summon her.                     │
│                                                                  │
│   ──────────────────────────────────────────────────────────────  │
│                                                                  │
│              ┌──────────────────────────────┐                    │
│              │           ⌘  ⌃               │                    │
│              │                              │                    │
│              │     [ Try it now ]           │  ← when pressed,   │
│              └──────────────────────────────┘    a mock chamber  │
│                                                  flashes on the  │
│              [ Use a different shortcut ]        screen (240ms)  │
│                                                                  │
│   ──────────────────────────────────────────────────────────────  │
│                                                                  │
│                                       [ Back ]    [ Continue ]   │
└──────────────────────────────────────────────────────────────────┘
```

- **Default**: `⌘⌃` shown as glyph pair, 32pt, ivory on `--obsidian-2` chip with gold-muted hairline.
- **Try it now**: pressing the chord live (anywhere on macOS) triggers a 240ms mock-chamber preview at screen-center — a Flourish-1 Gold Seam that opens, shows the words "I'm here.", and closes. Confirms muscle-memory works.
- **Re-record**: "Use a different shortcut" expands a recorder field that listens for the next key combo (Esc cancels). On capture: conflict check against macOS reserved shortcuts (`⌘Space`, `⌘Tab`, `⌘Q`, etc.) + existing user shortcuts (queried via `HIToolbox`). On conflict: red-alert caption "Conflicts with Spotlight. Try another." Field stays open until valid combo or "Cancel".
- **Modifier-only chord** (`⌘⌃` with no key) gets special handling: tap-to-summon (press + release within 250ms = trigger; hold = ignore). This matches Raycast's modifier-only convention. Display reads "Press and release ⌘⌃" to disambiguate.
- **Skip**: not present as a button — the recorder defaults to `⌘⌃` and "Continue" accepts that. Implicit skip.
- **Back**: returns to step 1 (no state lost).
- **Continue**: saves to `~/Library/Application Support/Leah/settings.json`, advances.

**Reference**: Raycast's modifier-only hotkey, Things' shortcut recorder, Arc's setup hotkey step.

### Step 3 — Voice + mic + wake-word (the value-first screen)

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ●  ●  ○  ○                                          step 3/5│
│  ──────────────────────────────────────────────────────────────  │
│                                                                  │
│   Talk to Leah.                                                  │
│   Say her name to wake her up — hands-free.                      │
│                                                                  │
│   ──────────────────────────────────────────────────────────────  │
│                                                                  │
│         ┌─────────────────────────────────────────────┐          │
│         │                                             │          │
│         │     ▏▎▍▌▋▊▉  Listening for "Leah"          │          │
│         │                                             │          │
│         └─────────────────────────────────────────────┘          │
│         (live waveform — only animates after mic granted)        │
│                                                                  │
│   ┌──────────────────────────────────────────────────────────┐   │
│   │ Microphone access                                        │   │
│   │ Needed to hear "Leah" and dictate questions.             │   │
│   │ Example: say "Leah, what's on my calendar today?"        │   │
│   │                                       [ Enable voice ]   │   │
│   └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│   ☑  Listen for "Leah" wake-word                                 │
│      You can change this anytime in Settings → Voice.            │
│                                                                  │
│   ──────────────────────────────────────────────────────────────  │
│                                                                  │
│                          [ Skip — text only ]                    │
│                                       [ Back ]    [ Continue ]   │
└──────────────────────────────────────────────────────────────────┘
```

- **Show-value-before-asking**: the waveform mock animates with a CANNED loop before grant (subtle, looped sine — labeled "(preview)" in `--text-dim` 10pt). On grant, switches to real mic input.
- **Enable voice button** → `AVCaptureDevice.requestAccess(.audio)` → triggers OS mic prompt. On grant: waveform goes live; card collapses to "✓ Microphone enabled". On deny: card shows "Microphone blocked in System Settings. [Open System Settings →] or [Continue without voice]". Both buttons work — no dead-end.
- **Wake-word toggle**: PRE-CHECKED (operator decision). Inline copy explains reversibility. Disabled (greyed) if mic not granted, with caption "Enable voice above to use wake-word".
- **Skip — text only**: equal-weight button, lower-left. Sets `voice.enabled = false`, `voice.wake_word = false`, advances. No guilt copy.
- **WHY copy is 1 line + 1 example** — Notion's permissions screen and Things' calendar prompt both use this exact pattern.
- **No mention of any other permission here.** Accessibility, screen-recording, automation: all deferred to Settings → Permissions (lazy-prompt on first use of the feature that needs them).

### Step 4 — One thing to connect (prove value)

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ●  ●  ●  ○                                          step 4/5│
│  ──────────────────────────────────────────────────────────────  │
│                                                                  │
│   Pick one thing for Leah to know about.                         │
│   You can connect more from Settings later.                      │
│                                                                  │
│   ──────────────────────────────────────────────────────────────  │
│                                                                  │
│   ◯ ┌──────────────────────────────────────────────────────┐    │
│     │ [📅] Calendar                                         │    │
│     │ "What's on today?" "Move my 3pm to tomorrow."         │    │
│     │ ≈ 20s · uses macOS Calendar (no OAuth)                │    │
│     └──────────────────────────────────────────────────────┘    │
│                                                                  │
│   ◯ ┌──────────────────────────────────────────────────────┐    │
│     │ [✉️ ] Mail                                             │    │
│     │ "Summarize unread." "Draft a reply to Sam."           │    │
│     │ ≈ 30s · OAuth via your default mail provider          │    │
│     └──────────────────────────────────────────────────────┘    │
│                                                                  │
│   ◯ ┌──────────────────────────────────────────────────────┐    │
│     │ [📁] Files                                            │    │
│     │ "Find my MAY-19 spec." "Open last week's notes."      │    │
│     │ ≈ 15s · pick one folder to index                      │    │
│     └──────────────────────────────────────────────────────┘    │
│                                                                  │
│   ──────────────────────────────────────────────────────────────  │
│                                                                  │
│           [ Skip — set up later ]    [ Back ]    [ Connect ]    │
└──────────────────────────────────────────────────────────────────┘
```

- **3 radio cards**, NOT 6. Calendar is pre-selected (highest-value default per ambient HUD's "Now" slot — operator sees value within 60s of finishing wizard).
- **Per-card copy**: name, two example utterances, time estimate, auth mechanism. Each is 1 truthful sentence; no "Boost your productivity" pap.
- **Connect button** changes label based on selected card: "Connect Calendar" / "Connect Mail" / "Choose folder". On click:
  - Calendar → `EKEventStore.requestFullAccessToEvents()` OS prompt → on grant, card collapses to "✓ Calendar connected — 12 events this week"; on deny, error sheet with [Open System Settings] + [Pick a different one].
  - Mail → opens default browser to OAuth flow; wizard shows "Waiting for browser…" with [Cancel]; on success, card collapses; on cancel/error, restores cards.
  - Files → opens NSOpenPanel for folder selection; on choose, background indexing starts (progress shown as a thin gold underline on the card, 0–100%); operator can advance immediately, indexing continues.
- **Skip** is left-justified, equal weight to Connect. Linear's onboarding pattern.
- **Why these three?** Calendar (most-frequent ambient surface), Mail (highest perceived value), Files (Spotlight-killer). One of these three serves any operator persona; the rest of the catalog (GitHub, Linear, Slack, Notion, arxiv, News, etc.) is in Settings.

### Step 5 — You're ready

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ●  ●  ●  ●                                          step 5/5│
│  ──────────────────────────────────────────────────────────────  │
│                                                                  │
│                                                                  │
│                              ⬡                                   │
│                                                                  │
│                       You're all set.                            │
│                                                                  │
│                  Press   ⌘  ⌃   to call me.                      │
│                       (any time, from any app)                   │
│                                                                  │
│                                                                  │
│   ──────────────────────────────────────────────────────────────  │
│                                                                  │
│        Tip: I'll live in the bottom-right of your screen.        │
│              Drag me anywhere you like.                          │
│                                                                  │
│        Need to add more — Mail, Slack, GitHub, Files?            │
│              [ Open Settings ]                                   │
│                                                                  │
│   ──────────────────────────────────────────────────────────────  │
│                                                                  │
│                                       [ Back ]      [ Done ]    │
└──────────────────────────────────────────────────────────────────┘
```

- **Hotkey reminder big** (32pt glyphs in a chip) — primary takeaway. Reference: Raycast's last-step hotkey reminder.
- **"Open Settings"** is a text-button (not a primary CTA) — invites discovery without forcing it.
- **"Done"** closes wizard and:
  1. Plays a single Flourish-1 Gold Seam at the bottom-right where the HUD will land (380ms).
  2. Ambient HUD materializes at default position.
  3. First toast appears above HUD: "Press ⌘⌃ to ask anything." (6s, dismissable). This is the post-onboarding "you can do it now" cue — Apple HIG calls it "show, don't tell".

## A.4 Error / edge-case matrix

| Scenario | Behavior | Anti-dead-end |
|---|---|---|
| Mic permission denied (step 3) | Card collapses to red-dim banner: "Microphone is blocked in System Settings." Two buttons: [Open System Settings] + [Continue without voice]. Both work; wake-word toggle disables silently. | No "you must enable mic to continue" trap. |
| Hotkey conflicts with macOS reserved (step 2) | Red-alert caption "Conflicts with Spotlight. Try another." Recorder stays open. Operator can cancel back to default. | Default `⌘⌃` is pre-validated; conflict only possible on re-record. |
| Calendar / Mail OAuth fails (step 4) | Card returns to un-selected state; toast: "Couldn't connect to Mail. [Try again] or pick another." | Cards remain selectable; Skip remains available. |
| Network down during integration step | Shows wifi glyph + "You're offline. Set up integrations later from Settings." Connect buttons disable; Skip remains active. | Wizard completes regardless. |
| Operator quits mid-wizard | App stays installed; relaunch resumes wizard at step 1 (fresh) — UNLESS step 3 mic was granted, then resumes at step 4 (forward progress preserved). | No state lost; no "complete setup" nag. |
| Operator re-runs wizard from Settings | Each step reflects current state. Step 3 shows "✓ Microphone already enabled" with toggle for wake-word. Step 4 shows current integrations with "Connect another" CTA. Step 2 shows current hotkey. | Wizard is idempotent. |
| Reduced motion ON | All Flourishes degrade per v1 §6.2 (cross-fades, no Gold Seam, no sigil tick). Progress dots update instantly. | Identical wizard, calmer. |
| VoiceOver active | Each step announces "Setup, step N of 5: [step name]." Focus lands on heading; Tab cycles heading → primary action → skip → back. | Fully operable without trackpad. |

## A.5 Anti-patterns explicitly avoided

| Anti-pattern (common in onboarding) | What we did instead |
|---|---|
| **Fake loading bar** ("Personalizing your experience…") | No artificial delays. Each step renders instantly; only real OS prompts have wait time. |
| **Confirmshaming** ("No thanks, I don't want a better experience") | Skip is neutral text: "Skip — set up later." No emotional asymmetry. |
| **Forced sequential gates** ("Grant all 8 permissions to continue") | Only mic is asked; the other 7+ are lazy-prompted at first-use in Settings. |
| **Pre-checked marketing opt-ins** | We pre-check exactly ONE thing: wake-word toggle (operator decision). Every other toggle is OFF or absent. |
| **Bait-and-switch wake-word** (showing "off" then enabling later) | Wake-word toggle is visible, on-screen, pre-checked, with one-click off + a link to Settings → Voice. |
| **Dark-pattern "Are you sure?" sheets** on skip | Skip is one click, no confirmation. Quit during wizard does confirm — but that's modal-quit convention, not anti-skip. |
| **Lengthy welcome video** | One sigil + one line + one TTS line of "Hi, I'm Leah". 380ms total. |
| **Pre-filled email/name fields harvested for marketing** | Wizard collects ZERO personal info. No email field. No "What should I call you?" — first usage of `leah ask` will pick up `$USER`. |
| **Multi-page T&C / "I agree" wall** | None. Privacy policy linked from step 5 only ("Open Settings → About"). |
| **Pop-up "Rate us" / "Send analytics" early** | Analytics opt-out lives in Settings → Privacy. Default ON or OFF is documented in v1.1 (this spec leaves it to product). |
| **Notification permission requested upfront** ("Let Leah send you notifications") | Deferred. Asked at first daemon-push event with inline rationale: "Leah wants to show brief-ready alerts. [Allow once] [Always allow] [Never]". |

---

# Part B — Settings preferences pane

## B.1 Window shape

- **Size**: 760 × 560pt default; resizable to 640 × 480pt min, no max.
- **Style**: Things-style — left sidebar (200pt), right detail pane (560pt+). Both columns scroll independently.
- **Chrome**: NSWindow `.titled` + `.fullSizeContentView`, traffic lights top-left at 20pt inset (matches focus chamber).
- **Search**: top of sidebar, full-width, `⌘F` focuses. Filters BOTH sidebar items AND detail-row keywords in real-time. Things' settings search is the gold standard — Leah copies it. Matches highlight gold-primary; non-matches dim to `--text-dim`.
- **Live preview pane**: Appearance section only — right-most 280pt of detail pane shows a miniature mock HUD + focus chamber that reacts to toggles in real-time. Arc browser does this for theme/sidebar; we copy.
- **Persistence**: window position + last-section sticky across launches.

## B.2 IA tree

```
Settings
├── General                ⌘1
│   ├── Start at login
│   ├── Hotkey (chord recorder + reset)
│   ├── HUD position (8-anchor grid + drag-anywhere note)
│   ├── HUD scale (Mini / Standard / Expanded)
│   ├── HUD visibility (always / focused-app only / Spaces filter)
│   └── HUD always-on-top
├── Voice                  ⌘2
│   ├── Wake-word toggle (on/off)
│   ├── Wake-word phrase ("Leah" / "Hey Leah" / "OK Leah")
│   ├── Voice model (3 TTS voices with hover-preview)
│   ├── Push-to-talk fallback (hold Space)
│   ├── Mute schedule (per-day quiet hours)
│   ├── Input language (auto / en / es / fr / …)
│   └── Output volume slider (with system-volume note)
├── Appearance             ⌘3
│   ├── Accent intensity (gold saturation slider; default 100%)
│   ├── Hairline opacity (4% / 8% / 12% / 16%; default 8%)
│   ├── Reduced motion (system / always)
│   ├── Density (compact / standard / spacious)
│   ├── Dock edge (alignment of HUD against screen edge)
│   ├── Grain overlay (system / on / off)
│   └── [Live preview pane on right]
├── Privacy                ⌘4
│   ├── Sensitive-content blocklist (regex + presets)
│   ├── Screen-recording auto-hide HUD (on by default)
│   ├── DND respect (on by default)
│   ├── Telemetry opt-out (1 toggle)
│   ├── Conversation logging (on / session-only / off)
│   └── Sensitive-app blocklist (1Password, banking, etc.)
├── Permissions            ⌘5
│   ├── Microphone               [green dot] Granted
│   ├── Accessibility            [gold dot]  Needed for [feature]
│   ├── Screen recording         [—]         Not asked
│   ├── Automation: Calendar     [red dot]   Denied — [Open System Settings]
│   ├── Automation: Mail         [—]         Not asked
│   ├── Contacts                 [—]         Not asked
│   ├── Reminders                [—]         Not asked
│   ├── Notifications            [—]         Not asked
│   ├── Focus filter integration [—]         Not asked
│   └── Full Disk Access         [—]         Not asked
├── Integrations           ⌘6
│   ├── Calendar (Apple)         [✓] Connected · 12 events this week  [Reconfigure] [Disconnect]
│   ├── Mail                     [—] Disconnected  [Connect]
│   ├── Files                    [✓] ~/Documents · 2,341 indexed  [Reconfigure] [Disconnect]
│   ├── Messages                 [—] Disconnected  [Connect]
│   ├── Reminders                [—] Disconnected  [Connect]
│   ├── Slack                    [—] Disconnected  [Connect]
│   ├── GitHub                   [✓] @trilam · 14 repos  [Reconfigure] [Disconnect]
│   ├── Linear                   [✓] Maydow · MAY team  [Reconfigure] [Disconnect]
│   ├── Notion                   [—] Disconnected  [Connect]
│   ├── arxiv                    [✓] cs.AI, cs.CL · 12 today  [Reconfigure] [Disconnect]
│   └── News                     [—] Disconnected  [Connect]
├── Memory                 ⌘7
│   ├── Storage location (path + Reveal in Finder)
│   ├── Total entries (e.g., "1,247 entries · 14.2 MB")
│   ├── Browse memory (opens dashboard → Memory tab)
│   ├── Export memory (JSON / NDJSON / Markdown)
│   ├── Import memory (JSON / NDJSON)
│   └── [Purge memory] — destructive, double-confirm
└── About                  ⌘8
    ├── Version (with "Check for updates")
    ├── Log location (path + Reveal in Finder)
    ├── Daemon status (green/gold/red dot + restart button)
    ├── Open-source licenses
    ├── Privacy policy
    ├── Support
    └── Re-run setup wizard
```

## B.3 Per-section wireframes

### B.3.1 General

```
┌────────────────────────────────────────────────────────────────────┐
│ [🔍 Search settings…]   │  General                                 │
│ ────────────────────────│  ──────────────────────────────────────  │
│ ● General               │                                          │
│ ○ Voice                 │   Start at login                         │
│ ○ Appearance            │   ☑                                      │
│ ○ Privacy               │   Launches Leah when you log in.         │
│ ○ Permissions           │                                          │
│ ○ Integrations          │   ──────────────────────────────────────  │
│ ○ Memory                │                                          │
│ ○ About                 │   Hotkey                                 │
│                         │   ┌──────────────┐                       │
│                         │   │    ⌘  ⌃      │  [ Re-record ] [Reset]│
│                         │   └──────────────┘                       │
│                         │   Press from any app to summon Leah.     │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Ambient HUD position                   │
│                         │   ┌────┐ ┌────┐ ┌────┐                  │
│                         │   │ TL │ │ T  │ │ TR │                  │
│                         │   └────┘ └────┘ └────┘                  │
│                         │   ┌────┐         ┌────┐                  │
│                         │   │ L  │   ⬡    │ R  │  ← preview        │
│                         │   └────┘         └────┘                  │
│                         │   ┌────┐ ┌────┐ ┌────┐                  │
│                         │   │ BL │ │ B  │ │●BR │  ← current        │
│                         │   └────┘ └────┘ └────┘                  │
│                         │   Drag the HUD anywhere; position saves. │
│                         │                                          │
│                         │   Scale     ◯ Mini  ●Standard  ◯ Expanded│
│                         │   Visibility   ●Always                   │
│                         │                ○ Hide in fullscreen apps │
│                         │                ○ Current Space only      │
│                         │   ☐  Always on top                       │
│                         │                                          │
└────────────────────────────────────────────────────────────────────┘
```

### B.3.2 Voice

```
┌────────────────────────────────────────────────────────────────────┐
│ [🔍 Search settings…]   │  Voice                                   │
│ ────────────────────────│  ──────────────────────────────────────  │
│ ○ General               │                                          │
│ ● Voice                 │   Wake word                              │
│ ○ Appearance            │   ☑                                      │
│ ○ Privacy               │   Say her name to summon her hands-free. │
│ ○ Permissions           │                                          │
│ ○ Integrations          │   Phrase     ●"Leah"                     │
│ ○ Memory                │              ○"Hey Leah"                 │
│ ○ About                 │              ○"OK Leah"                  │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Voice                                  │
│                         │   ●  Ivory  (warm soprano)    [▶ Sample] │
│                         │   ○  Walnut (warm baritone)   [▶ Sample] │
│                         │   ○  Slate  (neutral mid)     [▶ Sample] │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Push-to-talk fallback                  │
│                         │   ☑  Hold Space in focus chamber to talk │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Quiet hours                            │
│                         │   ☑  Mute Leah from  [22:00] to [07:00]  │
│                         │   Wake-word + voice responses suppressed.│
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Language                               │
│                         │   ▾ Auto-detect (current: English (US))  │
│                         │                                          │
│                         │   Output volume                          │
│                         │   ───●───────────────  60%               │
│                         │   Caps at your system volume.            │
│                         │                                          │
└────────────────────────────────────────────────────────────────────┘
```

### B.3.3 Appearance (with live preview pane)

```
┌────────────────────────────────────────────────────────────────────┐
│ [🔍 Search settings…]   │  Appearance                              │
│ ────────────────────────│  ──────────────────────────────────────  │
│ ○ General               │                                ┌───────┐ │
│ ○ Voice                 │  Accent intensity              │       │ │
│ ● Appearance            │  ──●─────  100%                │  ⬡    │ │
│ ○ Privacy               │  Saturation of gold accents.   │       │ │
│ ○ Permissions           │                                │ live  │ │
│ ○ Integrations          │  Hairline opacity              │ HUD   │ │
│ ○ Memory                │  ○ 4%  ● 8%  ○ 12%  ○ 16%      │ mock  │ │
│ ○ About                 │  Thin rules between rows.      │       │ │
│                         │                                │       │ │
│                         │  Density                       │       │ │
│                         │  ○ Compact  ●Standard  ○Spacious│      │ │
│                         │                                │       │ │
│                         │  Reduced motion                │       │ │
│                         │  ●  Follow system              │       │ │
│                         │  ○  Always reduce motion       │       │ │
│                         │                                │       │ │
│                         │  Grain overlay                 │       │ │
│                         │  ●  Follow system  ○ On  ○ Off │       │ │
│                         │  1.5% film grain on obsidian.  │       │ │
│                         │                                │       │ │
│                         │  Dock edge alignment           │       │ │
│                         │  ●  Snap to edge ○ Float free  │       │ │
│                         │                                └───────┘ │
└────────────────────────────────────────────────────────────────────┘
```

### B.3.4 Privacy

```
┌────────────────────────────────────────────────────────────────────┐
│ [🔍 Search settings…]   │  Privacy                                 │
│ ────────────────────────│  ──────────────────────────────────────  │
│ ○ General               │                                          │
│ ○ Voice                 │   Auto-hide during screen recording      │
│ ○ Appearance            │   ☑                                      │
│ ● Privacy               │   Hides HUD + suppresses toasts when     │
│ ○ Permissions           │   another app is recording your screen.  │
│ ○ Integrations          │                                          │
│ ○ Memory                │   ──────────────────────────────────────  │
│ ○ About                 │                                          │
│                         │   Respect Do Not Disturb                 │
│                         │   ☑                                      │
│                         │   Suppresses non-priority toasts when    │
│                         │   macOS Focus is on.                     │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Sensitive content blocklist            │
│                         │   ☑ Hide passwords, API keys, tokens     │
│                         │   ☑ Hide content from these apps:        │
│                         │      • 1Password        [ Remove ]       │
│                         │      • Banking          [ Remove ]       │
│                         │      [+ Add app]                         │
│                         │                                          │
│                         │   Custom regex                           │
│                         │   ┌──────────────────────────────────┐   │
│                         │   │ sk-[A-Za-z0-9]{32,}              │   │
│                         │   └──────────────────────────────────┘   │
│                         │   [+ Add pattern]                        │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Conversation logging                   │
│                         │   ●  Keep forever  (default)             │
│                         │   ○  Session only — clear on quit        │
│                         │   ○  Don't log                           │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Telemetry                              │
│                         │   ☐  Send anonymous usage stats          │
│                         │   Includes: crash counts, feature usage. │
│                         │   No conversation content. [What's sent?]│
│                         │                                          │
└────────────────────────────────────────────────────────────────────┘
```

### B.3.5 Permissions

```
┌────────────────────────────────────────────────────────────────────┐
│ [🔍 Search settings…]   │  Permissions                             │
│ ────────────────────────│  ──────────────────────────────────────  │
│ ○ General               │                                          │
│ ○ Voice                 │  ● Microphone                            │
│ ○ Appearance            │    Granted                               │
│ ○ Privacy               │    Hear wake-word and dictate questions. │
│ ● Permissions           │                                          │
│ ○ Integrations          │  ──────────────────────────────────────  │
│ ○ Memory                │                                          │
│ ○ About                 │  ◐ Accessibility                         │
│                         │    Needed for hotkey to work everywhere  │
│                         │    [ Grant in System Settings → ]        │
│                         │                                          │
│                         │  ──────────────────────────────────────  │
│                         │                                          │
│                         │  ○ Screen recording                      │
│                         │    Not asked yet                         │
│                         │    Needed only if you want Leah to read  │
│                         │    what's on screen. We'll ask the first │
│                         │    time you use that feature.            │
│                         │                                          │
│                         │  ──────────────────────────────────────  │
│                         │                                          │
│                         │  ✕ Automation: Calendar                  │
│                         │    Denied                                │
│                         │    Calendar features won't work until    │
│                         │    you allow Leah in System Settings.    │
│                         │    [ Open System Settings → ]            │
│                         │                                          │
│                         │  ──────────────────────────────────────  │
│                         │                                          │
│                         │  ○ Automation: Mail            [ Ask ]   │
│                         │  ○ Contacts                    [ Ask ]   │
│                         │  ○ Reminders                   [ Ask ]   │
│                         │  ○ Notifications               [ Ask ]   │
│                         │  ○ Focus filter integration    [ Ask ]   │
│                         │  ○ Full Disk Access            [ Ask ]   │
│                         │                                          │
└────────────────────────────────────────────────────────────────────┘
```

**Status glyph legend** (consistent everywhere):
- `●` green-quiet (`--success-quiet`) — granted
- `◐` gold (`--gold-primary`) — needs setup
- `○` open circle (`--text-muted`) — not asked yet
- `✕` red-alert (`--red-alert`) — denied / error

### B.3.6 Integrations

```
┌────────────────────────────────────────────────────────────────────┐
│ [🔍 Search settings…]   │  Integrations                            │
│ ────────────────────────│  ──────────────────────────────────────  │
│ ○ General               │                                          │
│ ○ Voice                 │  Connected                               │
│ ○ Appearance            │  ──────────────────────────────────────  │
│ ○ Privacy               │  ● Calendar (Apple)                      │
│ ○ Permissions           │    12 events this week                   │
│ ● Integrations          │    [ Reconfigure ]    [ Disconnect ]     │
│ ○ Memory                │                                          │
│ ○ About                 │  ● Files                                 │
│                         │    ~/Documents · 2,341 indexed           │
│                         │    [ Reconfigure ]    [ Disconnect ]     │
│                         │                                          │
│                         │  ● GitHub                                │
│                         │    @trilam · 14 repos                    │
│                         │    [ Reconfigure ]    [ Disconnect ]     │
│                         │                                          │
│                         │  ● Linear                                │
│                         │    Maydow · MAY team                     │
│                         │    [ Reconfigure ]    [ Disconnect ]     │
│                         │                                          │
│                         │  ● arxiv                                 │
│                         │    cs.AI, cs.CL · 12 today               │
│                         │    [ Reconfigure ]    [ Disconnect ]     │
│                         │                                          │
│                         │  ──────────────────────────────────────  │
│                         │  Available                               │
│                         │  ──────────────────────────────────────  │
│                         │  ○ Mail                  [ Connect ]     │
│                         │  ○ Messages              [ Connect ]     │
│                         │  ○ Reminders             [ Connect ]     │
│                         │  ○ Slack                 [ Connect ]     │
│                         │  ○ Notion                [ Connect ]     │
│                         │  ○ News                  [ Connect ]     │
│                         │                                          │
│                         │  ──────────────────────────────────────  │
│                         │  Errors                                  │
│                         │  ──────────────────────────────────────  │
│                         │  ✕ Slack                                 │
│                         │    Token expired 2 days ago              │
│                         │    [ Reconnect ]    [ Remove ]           │
│                         │                                          │
└────────────────────────────────────────────────────────────────────┘
```

- Sorted by status: Connected → Available → Errors. Operators see what's working before what's missing.
- "Reconfigure" opens the integration's configuration sheet (e.g., Files → folder picker; arxiv → category picker; Calendar → calendar-set picker).
- "Disconnect" is single-confirm: "Disconnect Calendar? Leah will lose access to your events. You can reconnect anytime." [Disconnect] [Cancel].

### B.3.7 Memory

```
┌────────────────────────────────────────────────────────────────────┐
│ [🔍 Search settings…]   │  Memory                                  │
│ ────────────────────────│  ──────────────────────────────────────  │
│ ○ General               │                                          │
│ ○ Voice                 │   Storage                                │
│ ○ Appearance            │   ~/Library/Application Support/Leah/    │
│ ○ Privacy               │     memory/                              │
│ ○ Permissions           │   [ Reveal in Finder ]                   │
│ ○ Integrations          │                                          │
│ ● Memory                │   ──────────────────────────────────────  │
│ ○ About                 │                                          │
│                         │   Contents                               │
│                         │   1,247 entries · 14.2 MB                │
│                         │   Oldest: 2026-04-12 · Newest: today     │
│                         │   [ Browse memory ] → opens Dashboard    │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Export                                 │
│                         │   [ Export as JSON ]                     │
│                         │   [ Export as Markdown ]                 │
│                         │                                          │
│                         │   Import                                 │
│                         │   [ Import from file… ]                  │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Purge memory                           │
│                         │   [ Purge all memory… ]   ← red-alert    │
│                         │                                          │
└────────────────────────────────────────────────────────────────────┘
```

**Purge confirmation flow** (double-confirm + count):

```
   ┌──────────────────────────────────────────────────┐
   │                                                  │
   │   Purge all memory?                              │
   │                                                  │
   │   1,247 entries will be permanently deleted.     │
   │   This cannot be undone.                         │
   │                                                  │
   │   Leah will forget every conversation, every     │
   │   note, every observation captured since         │
   │   2026-04-12.                                    │
   │                                                  │
   │   Type PURGE to confirm:                         │
   │   ┌──────────────────────────┐                   │
   │   │                          │                   │
   │   └──────────────────────────┘                   │
   │                                                  │
   │                       [ Cancel ]    [ Purge ]    │
   │                                  ↑               │
   │                                  disabled until  │
   │                                  text == "PURGE" │
   └──────────────────────────────────────────────────┘
```

GitHub-style typed-confirmation for irreversibility. Two friction points: count + typed word. Apple HIG calls this "destructive-action prevention". Linear uses this exact pattern for workspace deletion.

### B.3.8 About

```
┌────────────────────────────────────────────────────────────────────┐
│ [🔍 Search settings…]   │  About                                   │
│ ────────────────────────│  ──────────────────────────────────────  │
│ ○ General               │                                          │
│ ○ Voice                 │              ⬡                           │
│ ○ Appearance            │                                          │
│ ○ Privacy               │           Leah 0.5.0                     │
│ ○ Permissions           │     Build 2026-06-21 · main · 730bcd7    │
│ ○ Integrations          │                                          │
│ ○ Memory                │     [ Check for updates ]                │
│ ● About                 │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   ● Daemon running on :7331              │
│                         │   [ Restart daemon ]                     │
│                         │                                          │
│                         │   Log location                           │
│                         │   ~/Library/Logs/Leah/                   │
│                         │   [ Reveal in Finder ]    [ Copy path ]  │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   Open-source licenses                   │
│                         │   Privacy policy                         │
│                         │   Support · tri@maydow.com               │
│                         │   GitHub · github.com/treedesk/leah      │
│                         │                                          │
│                         │   ──────────────────────────────────────  │
│                         │                                          │
│                         │   [ Re-run setup wizard ]                │
│                         │                                          │
└────────────────────────────────────────────────────────────────────┘
```

## B.4 Toggle copy conventions

Every toggle has WHY in ≤1 line. No "Enable feature X" tautology.

- BAD: "Enable wake-word detection" → GOOD: "Say her name to summon her hands-free."
- BAD: "Telemetry" → GOOD: "Send anonymous usage stats. Includes crash counts, feature usage. No conversation content."
- BAD: "Auto-hide during screen recording" → GOOD: "Hides HUD + suppresses toasts when another app is recording your screen."
- BAD: "Start at login" → GOOD: "Launches Leah when you log in."

The 1-line rule: if you can't describe it in a sentence, the toggle shouldn't exist.

## B.5 Search behavior (Things-style)

- Sidebar field; full-width; `⌘F` focuses; Esc clears.
- Matches against: section names + every visible row label + every 1-line WHY caption + every keyword hint (e.g., "shortcut" matches Hotkey row).
- Behavior: as operator types, sections collapse to show only matching rows; non-matching sections hide entirely; matching text highlights `--gold-primary`. Empty result shows "No settings match 'foo'. [Search docs →]".
- Special: typing a known shortcut glyph (e.g., `⌘⌃`) jumps directly to the Hotkey row with it focused.

---

# C. Decision log

| # | Decision | Rationale |
|---|---|---|
| 1 | **5 wizard steps, not 6** | Folded HUD-position into Settings → General (operator drags HUD anyway; the picker is a discovery aid, not a setup gate). Saves 1 step, ~20s, and removes a decision with no wrong answer. |
| 2 | **Wake-word toggle co-located with mic prompt** | Wake-word IS a mic permission consequence. Splitting them makes the operator approve a thing they can't yet experience. Co-location preserves causality: grant mic → see waveform → toggle wake-word with full context. |
| 3 | **ONE integration in wizard, not 6** | Wizard length anxiety is the #1 abandonment cause (Linear, Granola, Cron data). 3 radio cards → 1 click. The other 9+ integrations are 1 menubar-click away in Settings → Integrations and self-discover via usage ("Connect Mail to ask about email"). |
| 4 | **Calendar is the pre-selected integration card** | Highest-frequency value: ambient HUD's "Now" slot needs an agenda source within 60s of finishing wizard, or the HUD looks dead. Mail and Files prove value too, but Calendar's value lands within the first minute regardless of usage. |
| 5 | **`⌘⌃` modifier-only chord** | Operator decision. Implementation: tap-to-summon (press + release <250ms). Raycast convention; well-tested muscle memory. Display copy explicitly says "Press and release ⌘⌃" to disambiguate from "hold ⌘⌃ + a key". |
| 6 | **Wake-word ON by default** | Operator decision (flips v1 designer's privacy-respectful OFF default). Mitigation: toggle is visible, on-screen, pre-checked, with a 1-click off + a link to Settings → Voice. Wake-word implementation must be local-only (no cloud audio) to retain trust. |
| 7 | **Permissions accumulate (lazy-prompt)** | Operator decision. Minimum-viable wizard asks only mic (load-bearing for wake-word demo). Other 7 permissions prompt on first use of the feature that needs them, with inline rationale. Avoids the "give me 9 permissions upfront" Mac-app-store smell. |
| 8 | **Settings search via Things-pattern** | Things' settings search is the highest-rated macOS-app settings UX (multiple App Store reviews single it out). It searches both section names AND row labels. Linear's settings has search but only filters sidebar — Things is the better pattern. |
| 9 | **Live preview pane ONLY on Appearance** | Arc has live preview throughout; Things has none. We pick Appearance only because it's the section where toggles have visual consequences. Voice / Privacy / Permissions toggles change behavior, not visuals — a preview would be theater. Avoids gratuitous preview chrome. |
| 10 | **Destructive memory-purge requires typed "PURGE"** | GitHub repo deletion + Linear workspace deletion pattern. Count + typed confirmation. Two friction points = effectively unaccidentable. Single-button confirm (e.g., a "Confirm" modal) is too easily muscle-memory-clicked. |
| 11 | **Status glyphs unified: ● green, ◐ gold, ○ open, ✕ red** | One legend across Permissions + Integrations + Daemon status. Consistency lets the operator scan state at a glance without a per-section legend. Color-blind safe: also distinguishable by shape. |
| 12 | **"Skip" is equal-weight, not a dim text-link** | Notion's onboarding inflates abandonment by making Skip a dim text-link. Linear gave up on this — their newer onboarding has equal-weight Skip + primary CTA. We follow Linear-new. |

---

# D. Reference UIs cited

| Choice | Reference |
|---|---|
| 5-step short wizard | Things first-run; Cron / Notion Calendar onboarding |
| Sigil hero + single CTA on welcome | Arc browser setup; Linear onboarding |
| Modifier-only hotkey + chord recorder | Raycast preferences |
| Live waveform mic preview before grant | Loom screen-recording onboarding; Granola mic permission |
| One-line WHY per permission | Things calendar/reminders permission cards; Notion first-run |
| Equal-weight Skip + Connect | Linear onboarding 2025-redesign; Cron integrations step |
| Settings sidebar + search | Things settings; Bear settings |
| Live preview pane | Arc theme/sidebar settings |
| Status glyph color system | Linear workspace settings (connected/disconnected/error) |
| Typed-confirmation for destructive action | GitHub repo delete; Linear workspace delete |
| Lazy-prompt permissions on first use | Apple HIG canonical; iOS app permission timing |
| Last-step hotkey reminder | Raycast final step; Things "you're set" screen |
| Toast first-cue post-wizard | Arc browser "press ⌘T to start" toast |
| Sigil acknowledgment animation | Inherits v1 §3.4 Flourish 2 |
| Gold Seam flourish on Done | Inherits v1 §3.4 Flourish 1 |

---

# E. Out of scope (deliberately deferred)

- **Multi-profile / multi-workspace setup** — Leah is single-operator personal. v1.1 if ever.
- **Cloud sync / account creation** — local-first; no account in v1.
- **Onboarding telemetry funnel** — measure later; ship the right UX first.
- **iCloud Keychain integration for integration tokens** — v1.1; v1 uses macOS Keychain locally.
- **Localized wizard** — v1 ships en-US only; copy is short enough to translate per release.
- **Onboarding email** — no email collected, so no welcome email. We do not import the operator's email address from `defaults read`.
- **HUD-position picker in wizard** — drag-anywhere is more flexible; the 8-anchor grid lives in Settings → General as a tap-to-snap convenience.

— end —
