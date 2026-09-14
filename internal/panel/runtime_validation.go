package panel

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// runtimeFieldError keeps recovery identity independent of the displayed text.
type runtimeFieldError struct {
	field string
	cause error
}

func (err *runtimeFieldError) Error() string { return err.cause.Error() }
func (err *runtimeFieldError) Unwrap() error { return err.cause }

func runtimeFieldFailure(field string, cause error) error {
	if field == "bot_config" {
		var setting *config.FieldError
		var wrongType *json.UnmarshalTypeError
		if errors.As(cause, &setting) {
			field += "." + setting.Field
		} else if errors.As(cause, &wrongType) && wrongType.Field != "" {
			field += "." + wrongType.Field
		}
	}
	// Alias entries belong to one atomic draft control. Preserve the detailed
	// path in the cause, while recovery identifies the editable collection.
	if strings.HasPrefix(field, "bot_config.command_aliases.") {
		field = "bot_config.command_aliases"
	}
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
