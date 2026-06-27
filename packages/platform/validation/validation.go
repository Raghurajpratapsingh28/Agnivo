// Package validation wraps go-playground/validator with stable, transport-
// friendly error reporting used across all executables.
package validation

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

// FieldError describes a single failed field.
type FieldError struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Error aggregates field errors from a failed validation.
type Error struct {
	Fields []FieldError `json:"fields"`
}

func (e *Error) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+": "+f.Rule)
	}
	return "validation failed: " + strings.Join(parts, ", ")
}

// Validator validates structs.
type Validator struct {
	v *validator.Validate
}

var (
	defaultOnce sync.Once
	defaultVal  *Validator
)

// New creates a validator that reports JSON field names rather than Go names
// and registers the platform's custom validation tags (slug, domain, url,
// docker_image, git_repo, env_var, secret) on top of validator's built-ins.
func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" || name == "" {
			return fld.Name
		}
		return name
	})
	registerCustom(v)
	return &Validator{v: v}
}

// registerCustom wires the platform's domain validators as struct tags. Each
// adapts a standalone predicate so the rules stay reusable outside struct
// validation too.
func registerCustom(v *validator.Validate) {
	tags := map[string]func(string) bool{
		"slug":         IsSlug,
		"domain":       IsDomain,
		"docker_image": IsDockerImage,
		"git_repo":     IsGitRepo,
		"env_var":      IsEnvVarName,
		"secret":       IsStrongSecret,
	}
	for tag, fn := range tags {
		fn := fn
		// Errors here are programmer errors (duplicate tags); panic surfaces
		// them at boot rather than silently dropping a rule.
		if err := v.RegisterValidation(tag, func(fl validator.FieldLevel) bool {
			return fn(fl.Field().String())
		}); err != nil {
			panic("validation: register " + tag + ": " + err.Error())
		}
	}
}

// Default returns a lazily-initialized shared validator.
func Default() *Validator {
	defaultOnce.Do(func() { defaultVal = New() })
	return defaultVal
}

// Struct validates s and returns a *Error on failure.
func (val *Validator) Struct(s any) error {
	err := val.v.Struct(s)
	if err == nil {
		return nil
	}
	var invalid *validator.InvalidValidationError
	if ok := asInvalid(err, &invalid); ok {
		return fmt.Errorf("validation: %w", err)
	}
	var verrs validator.ValidationErrors
	if !asValidationErrors(err, &verrs) {
		return err
	}
	out := &Error{Fields: make([]FieldError, 0, len(verrs))}
	for _, fe := range verrs {
		out.Fields = append(out.Fields, FieldError{
			Field:   fe.Field(),
			Rule:    fe.Tag(),
			Message: messageFor(fe),
		})
	}
	return out
}

func messageFor(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email"
	case "min":
		return "must be at least " + fe.Param()
	case "max":
		return "must be at most " + fe.Param()
	case "oneof":
		return "must be one of: " + fe.Param()
	case "uuid", "uuid4":
		return "must be a valid UUID"
	case "url":
		return "must be a valid URL"
	case "slug":
		return "must be a lowercase, hyphen-separated slug"
	case "domain":
		return "must be a valid domain name"
	case "docker_image":
		return "must be a valid Docker image reference"
	case "git_repo":
		return "must be a valid Git repository URL"
	case "env_var":
		return "must be a valid environment variable name"
	case "secret":
		return "must be a strong secret (min 16 chars, mixed character classes)"
	default:
		return "failed rule " + fe.Tag()
	}
}

func asInvalid(err error, target **validator.InvalidValidationError) bool {
	if e, ok := err.(*validator.InvalidValidationError); ok {
		*target = e
		return true
	}
	return false
}

func asValidationErrors(err error, target *validator.ValidationErrors) bool {
	if e, ok := err.(validator.ValidationErrors); ok {
		*target = e
		return true
	}
	return false
}
