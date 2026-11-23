package errors

import (
	stderrors "errors"
	"fmt"
)

// ActionableError identifies the operation that failed and whether retrying it
// is likely to succeed without changing the ConfigSync.
type ActionableError struct {
	Operation string
	Resource  string
	Retryable bool
	Err       error
}

func (e *ActionableError) Error() string {
	if e.Resource == "" {
		return fmt.Sprintf("%s failed: %v", e.Operation, e.Err)
	}
	return fmt.Sprintf("%s failed for %s: %v", e.Operation, e.Resource, e.Err)
}

func (e *ActionableError) Unwrap() error { return e.Err }

func Wrap(operation, resource string, retryable bool, err error) error {
	if err == nil {
		return nil
	}
	return &ActionableError{
		Operation: operation,
		Resource:  resource,
		Retryable: retryable,
		Err:       err,
	}
}

func IsRetryable(err error) bool {
	var actionable *ActionableError
	return stderrors.As(err, &actionable) && actionable.Retryable
}
