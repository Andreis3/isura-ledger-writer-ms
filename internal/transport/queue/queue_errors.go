package queue

import "time"

type TransientError struct {
	Err       error
	Delay     time.Duration
	SendToDLQ bool
}

func (e *TransientError) Error() string {
	return e.Err.Error()
}

func (e *TransientError) Unwrap() error {
	return e.Err
}

func NewTransientError(err error, delay time.Duration, sendToDLQ bool) *TransientError {
	return &TransientError{err, delay, sendToDLQ}
}

type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string {
	return e.Err.Error()
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}

func NewPermanentError(err error) *PermanentError {
	return &PermanentError{err}
}
