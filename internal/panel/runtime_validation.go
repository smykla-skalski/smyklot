package panel

import (
	"errors"
	"net/http"
)

// runtimeFieldError keeps recovery identity independent of the displayed text.
type runtimeFieldError struct {
	field string
	cause error
}

func (err *runtimeFieldError) Error() string { return err.cause.Error() }
func (err *runtimeFieldError) Unwrap() error { return err.cause }

func runtimeFieldFailure(field string, cause error) error {
	return &runtimeFieldError{field: field, cause: cause}
}

func writeRuntimeValidationError(w http.ResponseWriter, err error) {
	details := map[string]string{jsonFieldCode: "invalid_runtime_settings", jsonFieldMessage: err.Error()}
	var fieldError *runtimeFieldError
	if errors.As(err, &fieldError) {
		details["field"] = fieldError.field
	}
	writeJSON(w, http.StatusBadRequest, struct {
		Error map[string]string `json:"error"`
	}{Error: details})
}
