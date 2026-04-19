package internal

import "fmt"

type ExperimentEndCause int

const (
	ExperimentEndDueToTimeout         = ExperimentEndCause(1)
	ExperimentEndDueToScenarioSuccess = ExperimentEndCause(2)
	ExperimentEndDueToScenarioFailure = ExperimentEndCause(3)
	ExperimentEndDueToUnexpectedError = ExperimentEndCause(4)
)

type ExperimentEndError struct {
	CauseCode ExperimentEndCause
	comment   string
	cause     error
}

func (e ExperimentEndError) Error() string {
	str := fmt.Sprintf("experiment end error %s", e.String())
	if e.comment != "" {
		str = str + ": " + e.comment
	}
	if e.cause != nil {
		str = str + ":: " + e.cause.Error()
	}
	return str
}

func (e ExperimentEndError) Unwrap() error {
	return e.cause
}

func (e ExperimentEndError) String() string {
	switch e.CauseCode {
	case ExperimentEndDueToScenarioSuccess:
		return "scenario-success"
	case ExperimentEndDueToScenarioFailure:
		return "scenario-failure"
	case ExperimentEndDueToTimeout:
		return "scenario-timeout"
	case ExperimentEndDueToUnexpectedError:
		return "unexpected-error"
	default:
		return fmt.Sprintf("ExperimentEndError(%d)", e.CauseCode)
	}
}

func NewExperimentEndError(code ExperimentEndCause) *ExperimentEndError {
	return &ExperimentEndError{
		CauseCode: code,
	}
}
func NewExperimentEndErrorWithComment(code ExperimentEndCause, comment string) *ExperimentEndError {
	return &ExperimentEndError{
		CauseCode: code,
		comment:   comment,
	}
}
func NewExperimentEndErrorWithCause(code ExperimentEndCause, err error) *ExperimentEndError {
	return &ExperimentEndError{
		CauseCode: code,
		cause:     err,
	}
}
