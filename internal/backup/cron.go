// =============================================================================
// Bu dosya ne ise yarar?
//   Cron ifade dogrulama / sonraki calisma zamani yardimcilari.
// =============================================================================

package backup

import (
	"fmt"
)

// ValidateCron returns an error if the cron expression is empty.
func ValidateCron(expr string) error {
	if expr == "" {
		return fmt.Errorf("schedule must not be empty")
	}
	return nil
}
