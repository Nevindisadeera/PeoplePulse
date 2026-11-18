// Package httpx has the shared JSON response, error and request-decoding helpers.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"
)

// Error is an error that maps directly to an API error response.
type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func BadRequest(msg string) *Error { return &Error{Status: 400, Code: "BAD_REQUEST", Message: msg} }
func Unauthorized(msg string) *Error {
	return &Error{Status: 401, Code: "UNAUTHORIZED", Message: msg}
}
func Forbidden() *Error {
	return &Error{Status: 403, Code: "FORBIDDEN", Message: "You do not have permission to do this."}
}
func NotFound(what string) *Error {
	return &Error{Status: 404, Code: "NOT_FOUND", Message: what + " was not found."}
}
func Conflict(msg string) *Error { return &Error{Status: 409, Code: "CONFLICT", Message: msg} }
func TooMany() *Error {
	return &Error{Status: 429, Code: "RATE_LIMITED", Message: "Too many attempts. Try again later."}
}
func Invalid(fields map[string]string) *Error {
	return &Error{Status: 422, Code: "VALIDATION_FAILED", Message: "Some fields are invalid.", Fields: fields}
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// StatusClientClosedRequest is the conventional (non-standard) status recorded
// in the request log when the client went away before a response was written.
const StatusClientClosedRequest = 499

// Fail writes err as the standard error envelope. Unknown errors become a 500
// with a generic message; details go to the log only.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	reqID := middleware.GetReqID(r.Context())
	var e *Error
	if !errors.As(err, &e) {
		// The client closed the connection (for example, the user changed a
		// filter while a request was in flight). Nothing went wrong on the
		// server and nobody is waiting for a response.
		if r.Context().Err() != nil || errors.Is(err, context.Canceled) {
			w.WriteHeader(StatusClientClosedRequest)
			return
		}
		slog.Error("unhandled error", "requestId", reqID, "path", r.URL.Path, "err", err)
		e = &Error{Status: 500, Code: "INTERNAL", Message: "Something went wrong. Quote the request ID if you report this."}
	}
	JSON(w, e.Status, map[string]any{"error": map[string]any{
		"code": e.Code, "message": e.Message, "fields": e.Fields, "requestId": reqID,
	}})
}

var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		return strings.Split(f.Tag.Get("json"), ",")[0]
	})
	return v
}()

// Decode reads a JSON body into dst and runs struct validation.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return BadRequest("Request body is not valid JSON for this endpoint.")
	}
	if err := validate.Struct(dst); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			fields := map[string]string{}
			for _, fe := range ve {
				fields[fe.Field()] = describe(fe)
			}
			return Invalid(fields)
		}
		return err
	}
	return nil
}

func describe(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return "must be at least " + fe.Param() + " long"
	case "max":
		return "must be at most " + fe.Param() + " long"
	case "oneof":
		return "must be one of: " + fe.Param()
	}
	return "is invalid"
}

// Page reads page and pageSize query parameters with sane bounds.
func Page(r *http.Request) (page, pageSize int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ = strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}
