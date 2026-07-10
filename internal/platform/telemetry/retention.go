package telemetry

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRetentionDays = 30
	gzipAfterDays        = 7
)

// RetentionDays reads LEAH_AUDIT_RETENTION_DAYS; bad/non-positive → default.
func RetentionDays() int {
	if v, err := strconv.Atoi(os.Getenv("LEAH_AUDIT_RETENTION_DAYS")); err == nil && v > 0 {
		return v
	}
	return defaultRetentionDays
}

// PruneLogs gzips rotator files older than a week and deletes those past
// retentionDays; the live current-day file (today's UTC date) is skipped.
func PruneLogs(dir string, now time.Time, retentionDays int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	today := now.UTC().Truncate(24 * time.Hour)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		day, ok := parseLogDate(name)
		if !ok || !day.Before(today) {
			continue
		}
		ageDays := int(today.Sub(day).Hours() / 24)
		path := filepath.Join(dir, name)
		if ageDays >= retentionDays {
			if err := os.Remove(path); err != nil {
				return err
			}
			continue
		}
		if ageDays >= gzipAfterDays && strings.HasSuffix(name, ".jsonl") {
			if err := gzipInPlace(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseLogDate(name string) (time.Time, bool) {
	if !strings.HasPrefix(name, "leah-") {
		return time.Time{}, false
	}
	rest := strings.TrimPrefix(name, "leah-")
	rest = strings.TrimSuffix(strings.TrimSuffix(rest, ".gz"), ".jsonl")
	t, err := time.Parse("2006-01-02", rest)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// gzipInPlace compresses to a .gz.tmp then renames, removing the original
// only after the .gz is in place, so a crash mid-compress loses nothing.
func gzipInPlace(src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	tmp := src + ".gz.tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	if _, err := io.Copy(zw, in); err != nil {
		_, _ = zw.Close(), out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := zw.Close(); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, src+".gz"); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Remove(src)
}
