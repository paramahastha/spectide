package cli

import (
	"errors"
	"fmt"
)

const (
	ExitSuccess   = 0
	ExitCommand   = 2
	ExitBlocked   = 3
	ExitFailed    = 4
	ExitCancelled = 5
)

type CommandError struct {
	Code int
	Err  error
}

func (e *CommandError) Error() string {
	if e == nil || e.Err == nil {
		return "command failed"
	}
	return e.Err.Error()
}

func (e *CommandError) Unwrap() error { return e.Err }

func commandError(code int, message string) error {
	return &CommandError{Code: code, Err: fmt.Errorf("%s", message)}
}

// ExitCode maps a returned CLI error to a stable process status. Unknown
// command errors default to the command/usage status.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	var coded *CommandError
	if errors.As(err, &coded) && coded.Code >= ExitCommand && coded.Code <= ExitCancelled {
		return coded.Code
	}
	return ExitCommand
}
