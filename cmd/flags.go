package cmd

import (
	"errors"
	"strings"
)

// ErrorEmpty returned when value is empty.
var ErrorEmpty = errors.New("empty value not allowed")

// StringSlice is a flag that allows to set multiple types of string
type StringSlice []string

func (s *StringSlice) String() string {
	return "string slice"
}

// Set unmarshals enode into discv5.Node.
func (s *StringSlice) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 {
		return ErrorEmpty
	}
	*s = append(*s, trimmed)
	return nil
}
