package backup

import (
	"sort"
	"time"

	"github.com/diclebolek/Safepoint/internal/storage"
)

// ApplyRetention returns object keys older than retentionDays.
// Objects without LastModified are skipped.
func ApplyRetention(objects []storage.ObjectInfo, retentionDays int32, now time.Time) []string {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := now.UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	var toDelete []string
	for _, obj := range objects {
		if obj.LastModified.IsZero() {
			continue
		}
		if obj.LastModified.Before(cutoff) {
			toDelete = append(toDelete, obj.Key)
		}
	}
	sort.Strings(toDelete)
	return toDelete
}
