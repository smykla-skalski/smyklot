package config

// FieldError identifies a rejected setting without parsing its message.
// Wrapping retains the original error identity for errors.Is and errors.As.
type FieldError struct {
	Field string
	Cause error
}

func (err *FieldError) Error() string { return err.Cause.Error() }
func (err *FieldError) Unwrap() error { return err.Cause }
