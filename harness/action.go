package harness

import "fmt"

// ErrWhitelistViolation is returned by ReasoningContext.Publish when the
// requested subject is not in the imp's declared Actions whitelist. The
// message does not reach NATS.
type ErrWhitelistViolation struct {
	Subject string
}

func (e *ErrWhitelistViolation) Error() string {
	return fmt.Sprintf("harness: publish to subject %q rejected (not in whitelist)", e.Subject)
}
