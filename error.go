package walky

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type YAMLError struct {
	Line     int
	Column   int
	Filename string
	Context  string
	Err      error
}

func NewYAMLError(err error, node *yaml.Node) error {
	tmp := ErrDecode(err)
	if ye, ok := tmp.(YAMLError); ok {
		ye.Line = node.Line
		ye.Column = node.Column
		ye.Context = node.Value
		return ye
	}
	return YAMLError{
		Line:    node.Line,
		Column:  node.Column,
		Context: node.Value,
		Err:     err,
	}
}

func (e YAMLError) Error() string {
	return e.location() + ": " + e.Err.Error()
}

func (e YAMLError) location() string {
	var msg strings.Builder
	if e.Line > 0 {
		if e.Filename != "" {
			msg.WriteString(e.Filename + ":")
		} else {
			msg.WriteString("line ")
		}
		msg.WriteString(strconv.Itoa(e.Line))
		if e.Column > 0 {
			msg.WriteString(":" + strconv.Itoa(e.Column))
		}
	} else if e.Filename != "" {
		msg.WriteString(e.Filename)
	}
	if e.Context != "" {
		msg.WriteString(fmt.Sprintf(" at %q", e.Context))
	}
	return msg.String()
}

func (e YAMLError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			io.WriteString(s, e.location())
			fmt.Fprintf(s, ": %+v", e.Err)
			return
		}
		fallthrough
	case 's':
		io.WriteString(s, e.Error())
	case 'q':
		fmt.Fprintf(s, "%q", e.Error())
	}
}

func (e YAMLError) Unwrap() error {
	return e.Err
}

func ErrFilename(err error, filename string) error {
	tmp := ErrDecode(err)

	// short circuit if err is already YAMLError
	if ye, ok := tmp.(YAMLError); ok {
		ye.Filename = filename
		return ye
	}

	ye := YAMLError{}
	if errors.As(tmp, &ye) {
		if ye.Filename == filename {
			// file already set, so return original
			return err
		}
		ye.Filename = filename
		ye.Err = err
		return ye
	}
	return YAMLError{
		Filename: filename,
		Err:      err,
	}
}

func ErrDecode(err error) error {
	// short circuit if err is already YAMLError
	ye := YAMLError{}
	if errors.As(err, &ye) {
		return err
	}

	if te, ok := err.(*yaml.TypeError); ok {
		if len(te.Errors) > 1 {
			return err
		}
		if strings.HasPrefix(te.Errors[0], "line ") {
			line, msg, _ := strings.Cut(te.Errors[0], ": ")
			line = strings.TrimPrefix(line, "line ")
			lineNo, convErr := strconv.Atoi(line)
			if convErr != nil {
				return err
			}
			return YAMLError{
				Line: lineNo,
				Err:  errors.New(msg),
			}
		}
	}
	return err
}
