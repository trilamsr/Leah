package brief

import (
	"fmt"
	"strings"
)

// Render turns Data into a markdown brief. Pure function — drives the test
// suite.
func Render(d Data) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# leah brief — %s\n\n", d.Now.Format("Mon Jan 2 2006"))

	// 1. Yesterday recap.
	fmt.Fprintln(&b, "## Yesterday")
	total := 0
	for _, n := range d.YesterdayActions {
		total += n
	}
	if total == 0 {
		fmt.Fprintln(&b, "  (no leah actions logged)")
	} else {
		parts := []string{}
		for _, k := range []string{"ask", "ship", "review", "status"} {
			if n := d.YesterdayActions[k]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, k))
			}
		}
		fmt.Fprintf(&b, "  %s — $%.4f spent\n", strings.Join(parts, ", "), d.YesterdaySpend)
	}
	fmt.Fprintln(&b)

	// 2. Today's regatta backlog (top 3 active agents).
	fmt.Fprintln(&b, "## Regatta backlog")
	if len(d.ActiveAgents) == 0 {
		fmt.Fprintln(&b, "  (no active agents)")
	} else {
		max := 3
		if len(d.ActiveAgents) < max {
			max = len(d.ActiveAgents)
		}
		for _, a := range d.ActiveAgents[:max] {
			id := a.ID
			if len(id) > 12 {
				id = id[:12]
			}
			pr := "-"
			if a.PR > 0 {
				pr = fmt.Sprintf("PR #%d", a.PR)
			}
			fmt.Fprintf(&b, "  - %s %s (%s) %s\n", id, a.Branch, a.State, pr)
		}
		if len(d.ActiveAgents) > 3 {
			fmt.Fprintf(&b, "  …and %d more\n", len(d.ActiveAgents)-3)
		}
	}
	fmt.Fprintln(&b)

	// 3. Operator recommendations.
	fmt.Fprintln(&b, "## Recommendations")
	if !d.ModelReady {
		fmt.Fprintln(&b, "  (operator-model not ready yet — need more audit history)")
	} else if len(d.Recommendations) == 0 {
		fmt.Fprintln(&b, "  (no recommendations for this hour)")
	} else {
		for i, r := range d.Recommendations {
			fmt.Fprintf(&b, "  %d. %s — %s\n", i+1, r.Kind, r.Reason)
		}
	}
	fmt.Fprintln(&b)

	// 4. Bug-fix candidates.
	fmt.Fprintln(&b, "## Bug-fix candidates")
	if d.BugFixCount == 0 {
		fmt.Fprintln(&b, "  (none queued)")
	} else {
		fmt.Fprintf(&b, "  %d queued — see ~/.leah-state/bug-fix-candidates.md\n", d.BugFixCount)
	}
	fmt.Fprintln(&b)

	// 5. Feeds + Mail + Calendar render only when the operator has wired
	// the integration — silent absence beats noisy "unavailable" for
	// unconfigured features (UnavailableX is for runtime failure). Spec-
	// pinned order: weather → calendar → mail → news → market.
	renderWeather(&b, d)

	renderCalendar(&b, d)
	renderMail(&b, d)

	renderWorkSection(&b, "Jira", d.JiraItems, d.JiraUnavailable)
	renderWorkSection(&b, "Confluence", d.ConfluenceItems, d.ConfluenceUnavailable)

	renderNews(&b, d)
	renderMarket(&b, d)
	renderWatchlist(&b, d)

	// 6. Cost outlook.
	fmt.Fprintln(&b, "## Cost")
	fmt.Fprintf(&b, "  week-to-date  $%.4f\n", d.WeekToDateUSD)
	fmt.Fprintf(&b, "  projected mo  $%.4f\n", d.ProjectedMonthly)

	return b.String()
}

// renderCalendar mirrors the feeds pattern: silent when unwired, "(unavailable)"
// on runtime failure, else the capped event list with an overflow marker.
func renderCalendar(b *strings.Builder, d Data) {
	if !d.CalendarUnavailable && len(d.TodayEvents) == 0 {
		return
	}
	fmt.Fprintln(b, "## Calendar")
	if d.CalendarUnavailable {
		fmt.Fprintln(b, "  (unavailable)")
		fmt.Fprintln(b)
		return
	}
	max := gcalCap
	if len(d.TodayEvents) < max {
		max = len(d.TodayEvents)
	}
	fmt.Fprintf(b, "  %d events\n", len(d.TodayEvents))
	for _, ev := range d.TodayEvents[:max] {
		fmt.Fprintf(b, "  - %s %s\n", ev.Start.Format("15:04"), ev.Summary)
	}
	if len(d.TodayEvents) > max {
		fmt.Fprintf(b, "  …and %d more\n", len(d.TodayEvents)-max)
	}
	fmt.Fprintln(b)
}

// renderMail mirrors renderCalendar; UnreadMailTotal falls back to slice len
// when the adapter didn't report a total (pre-cap count).
func renderMail(b *strings.Builder, d Data) {
	if !d.MailUnavailable && d.UnreadMailTotal == 0 && len(d.UnreadMail) == 0 {
		return
	}
	fmt.Fprintln(b, "## Mail")
	if d.MailUnavailable {
		fmt.Fprintln(b, "  (unavailable)")
		fmt.Fprintln(b)
		return
	}
	total := d.UnreadMailTotal
	if total == 0 {
		total = len(d.UnreadMail)
	}
	shown := len(d.UnreadMail)
	if shown > gmailCap {
		shown = gmailCap
	}
	fmt.Fprintf(b, "  %d unread\n", total)
	for _, s := range d.UnreadMail[:shown] {
		fmt.Fprintf(b, "  - %s\n", s)
	}
	if total > shown {
		fmt.Fprintf(b, "  …and %d more\n", total-shown)
	}
	fmt.Fprintln(b)
}

// renderWorkSection mirrors the mail/calendar gating: silent when the tool
// is unconnected (no items, not unavailable), "(unavailable)" on runtime
// failure, else the capped item list with an overflow marker.
func renderWorkSection(b *strings.Builder, name string, items []WorkItem, unavailable bool) {
	if !unavailable && len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n", name)
	if unavailable {
		fmt.Fprintln(b, "  (unavailable)")
		fmt.Fprintln(b)
		return
	}
	max := workCap
	if len(items) < max {
		max = len(items)
	}
	fmt.Fprintf(b, "  %d items\n", len(items))
	for _, it := range items[:max] {
		if it.Detail != "" {
			fmt.Fprintf(b, "  - %s — %s\n", it.Title, it.Detail)
		} else {
			fmt.Fprintf(b, "  - %s\n", it.Title)
		}
	}
	if len(items) > max {
		fmt.Fprintf(b, "  …and %d more\n", len(items)-max)
	}
	fmt.Fprintln(b)
}

// VoiceSummary is the 1-sentence form spoken when TTS notify is wired. TTS
// on the full markdown is painful.
func VoiceSummary(d Data) string {
	yTotal := 0
	for _, n := range d.YesterdayActions {
		yTotal += n
	}
	return fmt.Sprintf("%d actions yesterday, %d active agents, %d bug-fix candidates, $%.2f week to date.",
		yTotal, len(d.ActiveAgents), d.BugFixCount, d.WeekToDateUSD)
}
