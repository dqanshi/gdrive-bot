package utils

import (
	"fmt"
	"strings"
	"time"
)

// HumanBytes renders a byte count as e.g. "512.0 MB", matching the units
// users expect from download/upload progress messages.
func HumanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := "KMGTPE"
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), units[exp])
}

// HumanSpeed renders bytes/sec as e.g. "3.2 MB/s".
func HumanSpeed(bytesPerSec float64) string {
	return HumanBytes(int64(bytesPerSec)) + "/s"
}

// HumanDuration renders a duration as e.g. "1h 4m 12s", trimming leading
// zero units. Used for ETA and uptime.
func HumanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	var parts []string
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if h > 0 || m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	parts = append(parts, fmt.Sprintf("%ds", s))
	return strings.Join(parts, " ")
}

// ProgressBar renders a text progress bar like "[████████░░░░░░░░] 52%",
// used in download/upload status messages edited in place.
func ProgressBar(done, total int64, width int) string {
	if width <= 0 {
		width = 16
	}
	var pct float64
	if total > 0 {
		pct = float64(done) / float64(total)
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(width))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("[%s] %.0f%%", bar, pct*100)
}

// TruncateMiddle shortens long filenames for inline keyboard buttons while
// keeping the extension visible, e.g. "Some.Very.Long.Movie.Name...mkv".
func TruncateMiddle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 5 {
		return s[:maxLen]
	}
	keep := maxLen - 3
	head := keep / 2
	tail := keep - head
	return s[:head] + "..." + s[len(s)-tail:]
}
