package main

import (
	"errors"
	"fmt"

	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/format"
	childprocess "github.com/Lensi-zhang/Akey-CLI/internal/process"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

func errorExitCode(err error) int {
	if err == nil {
		return 0
	}
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		if code := coded.ExitCode(); code >= 0 {
			return code
		}
	}
	return apperr.ExitCode(err)
}

func (application *application) renderError(err error) {
	var childExit *childprocess.ExitError
	if errors.As(err, &childExit) {
		return
	}

	body := errorBody{Code: "runtime", Message: err.Error()}
	var applicationError *apperr.Error
	if errors.As(err, &applicationError) {
		body.Code = applicationError.Code
		body.Message = applicationError.Message
		body.Hint = applicationError.Hint
	}
	if application.output == "json" {
		encoded, encodeErr := format.JSON(errorResponse{Error: body})
		if encodeErr == nil {
			application.printer.data(encoded)
			return
		}
	}
	if application.printer == nil {
		application.refreshPrinter()
	}
	_, _ = fmt.Fprintf(application.stderr, "✗ %s\n", body.Message)
	if body.Hint != "" && !application.nonInteractive {
		_, _ = fmt.Fprintf(application.stderr, "ℹ %s\n", body.Hint)
	}
}
