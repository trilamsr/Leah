// Package store is the strategist's maildir-style queue: one markdown
// file per item, atomic os.Rename for every state transition. The
// filesystem layout (queue/inbox/sent/killed) IS the audit log — no
// secondary index, no SQLite, no goroutine-owned mutex. Operators
// debug with ls/cat/vim.
package store

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SchemaCurrent is the only schema this loader accepts on read. v2
// fields bump this and the loader gains a case — old items stay
// parsable without migration.
const SchemaCurrent = 1

// ErrEmptyQueue signals Next found no queued items. Distinct from a
// filesystem error so the CLI can map it to exit code 3.
var ErrEmptyQueue = errors.New("strategist store: queue empty")

// Item is one queued / inboxed / sent / killed post. Schema is mandatory
// on write — a zero value is rejected so a struct literal that forgets
// the field fails loudly instead of writing schema:0 to disk.
type Item struct {
	Schema   int       // mandatory; must equal SchemaCurrent
	ID       string    // ULID, lexicographic + monotonic
	Channel  string    // linkedin | instagram | facebook | tiktok
	Slot     string    // commit | voice | news
	Text     string    // post body
	Origin   string    // git sha / voice path / news URL — traceability
	ImageRef string    // absolute path; "" when image-less
	ClipRef  string    // absolute path; "" when clip-less
	Created  time.Time
}

// MailDir is the on-disk state machine. root is <configDir>/strategist/.
type MailDir struct{ root string }

// Open ensures the four state directories exist under root and returns
// the handle. Idempotent — safe to call on every CLI invocation.
func Open(root string) (*MailDir, error) {
	for _, sub := range []string{"queue", "inbox", "sent", "killed"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return nil, fmt.Errorf("strategist store mkdir %s: %w", sub, err)
		}
	}
	return &MailDir{root: root}, nil
}

// Enqueue writes the item to queue/<id>.md using O_CREATE|O_EXCL — a
// pre-existing ID is an error (logic bug or ULID collision), never a
// silent overwrite.
func (m *MailDir) Enqueue(it Item) error { return m.writeNew("queue", it) }

// Inbox writes the item to inbox/<id>.md with the same exclusivity
// contract as Enqueue. The CLI uses inbox/ for soft-failed posts the
// operator should review before promoting to queue/.
func (m *MailDir) Inbox(it Item) error { return m.writeNew("inbox", it) }

// Next returns the oldest queued item by filename (ULIDs sort
// lexicographically by creation time). It does NOT move the file —
// the caller is required to call Sent or Killed to advance state.
// Two racing Nexts can observe the same item; Sent is the atomic seam.
func (m *MailDir) Next() (Item, error) {
	entries, err := readDirSorted(filepath.Join(m.root, "queue"))
	if err != nil {
		return Item{}, err
	}
	if len(entries) == 0 {
		return Item{}, ErrEmptyQueue
	}
	return readItem(filepath.Join(m.root, "queue", entries[0]))
}

// Sent atomically renames queue/<id>.md → sent/<id>.md. The OS rename
// guarantees exactly-one-success across concurrent callers — the
// loser sees os.ErrNotExist.
func (m *MailDir) Sent(id string) error { return m.rename("queue", "sent", id) }

// Killed atomically renames the item from any source dir to killed/.
// Used both for compose-time hard fails (write fresh) and for operator
// `inbox kill` (rename from inbox/ or queue/).
func (m *MailDir) Killed(id string) error {
	// Try the three live dirs in the order the CLI is most likely to
	// invoke from — inbox is by far the hot path for operator kills.
	for _, from := range []string{"inbox", "queue", "sent"} {
		err := m.rename(from, "killed", id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return fmt.Errorf("strategist store kill %s: %w", id, os.ErrNotExist)
}

// ListQueue returns every item currently in queue/, oldest first.
// A single corrupt file fails the whole call — better to surface the
// problem than to silently drop entries.
func (m *MailDir) ListQueue() ([]Item, error) { return m.list("queue") }

// ListInbox returns every item currently in inbox/, oldest first.
func (m *MailDir) ListInbox() ([]Item, error) { return m.list("inbox") }

// writeNew serializes the item and writes it with O_CREATE|O_EXCL.
func (m *MailDir) writeNew(dir string, it Item) error {
	if it.Schema != SchemaCurrent {
		return fmt.Errorf("strategist store: schema %d not supported (expect %d)", it.Schema, SchemaCurrent)
	}
	if it.ID == "" {
		return errors.New("strategist store: empty ID")
	}
	path := filepath.Join(m.root, dir, it.ID+".md")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("strategist store %s/%s: %w", dir, it.ID, err)
	}
	defer f.Close()
	if _, err := io.WriteString(f, serialize(it)); err != nil {
		return fmt.Errorf("strategist store write %s: %w", path, err)
	}
	return nil
}

// rename runs os.Rename — the OS guarantees atomicity across the
// transition, so this is the load-bearing primitive for every state
// change after creation.
func (m *MailDir) rename(from, to, id string) error {
	src := filepath.Join(m.root, from, id+".md")
	dst := filepath.Join(m.root, to, id+".md")
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("strategist store rename %s→%s: %w", from, to, err)
	}
	return nil
}

func (m *MailDir) list(dir string) ([]Item, error) {
	names, err := readDirSorted(filepath.Join(m.root, dir))
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(names))
	for _, n := range names {
		it, err := readItem(filepath.Join(m.root, dir, n))
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, nil
}

func readDirSorted(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("strategist store readdir %s: %w", dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

// serialize is the inverse of readItem. Schema 1 front-matter shape per
// docs/superpowers/specs/2026-06-21-leah-social-strategist-design.md §
// "Queue item markdown shape".
func serialize(it Item) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "schema: %d\n", it.Schema)
	fmt.Fprintf(&b, "id: %s\n", it.ID)
	fmt.Fprintf(&b, "channel: %s\n", it.Channel)
	fmt.Fprintf(&b, "slot: %s\n", it.Slot)
	fmt.Fprintf(&b, "origin: %s\n", it.Origin)
	if it.ImageRef != "" {
		fmt.Fprintf(&b, "image: %s\n", it.ImageRef)
	}
	if it.ClipRef != "" {
		fmt.Fprintf(&b, "clip: %s\n", it.ClipRef)
	}
	fmt.Fprintf(&b, "created: %s\n", it.Created.Format(time.RFC3339))
	b.WriteString("---\n\n")
	b.WriteString(it.Text)
	return b.String()
}

// readItem parses the front-matter + body. It rejects any schema value
// other than SchemaCurrent with an "unknown schema" error so a v2 file
// scanned by a v1 binary never silently misparses.
func readItem(path string) (Item, error) {
	f, err := os.Open(path)
	if err != nil {
		return Item{}, fmt.Errorf("strategist store open %s: %w", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	// First line must be "---".
	if !sc.Scan() || sc.Text() != "---" {
		return Item{}, fmt.Errorf("strategist store %s: missing front-matter open", path)
	}
	var it Item
	for sc.Scan() {
		line := sc.Text()
		if line == "---" {
			break
		}
		key, val, ok := splitKV(line)
		if !ok {
			return Item{}, fmt.Errorf("strategist store %s: malformed front-matter line %q", path, line)
		}
		switch key {
		case "schema":
			n, err := strconv.Atoi(val)
			if err != nil {
				return Item{}, fmt.Errorf("strategist store %s: schema not int: %w", path, err)
			}
			if n != SchemaCurrent {
				return Item{}, fmt.Errorf("strategist store %s: unknown schema %d (this binary supports %d)", path, n, SchemaCurrent)
			}
			it.Schema = n
		case "id":
			it.ID = val
		case "channel":
			it.Channel = val
		case "slot":
			it.Slot = val
		case "origin":
			it.Origin = val
		case "image":
			it.ImageRef = val
		case "clip":
			it.ClipRef = val
		case "created":
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return Item{}, fmt.Errorf("strategist store %s: created not RFC3339: %w", path, err)
			}
			it.Created = t
		}
	}
	if it.Schema != SchemaCurrent {
		return Item{}, fmt.Errorf("strategist store %s: missing schema field", path)
	}
	// Body is the rest, with the one blank line after "---" trimmed.
	var body strings.Builder
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first && line == "" {
			first = false
			continue
		}
		first = false
		body.WriteString(line)
		body.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return Item{}, fmt.Errorf("strategist store scan %s: %w", path, err)
	}
	it.Text = body.String()
	return it, nil
}

func splitKV(line string) (k, v string, ok bool) {
	i := strings.Index(line, ":")
	if i <= 0 {
		return "", "", false
	}
	return line[:i], strings.TrimSpace(line[i+1:]), true
}
