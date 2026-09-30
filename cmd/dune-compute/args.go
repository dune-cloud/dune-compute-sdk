// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type usageFailure struct {
	message string
}

func (e usageFailure) Error() string { return e.message }

func isFlag(arg string) bool {
	return strings.HasPrefix(arg, "--") || arg == "-h"
}

func flagParts(arg string) (string, string, bool) {
	if !strings.HasPrefix(arg, "--") || arg == "--" {
		return "", "", false
	}
	body := strings.TrimPrefix(arg, "--")
	name, value, ok := strings.Cut(body, "=")
	if name == "" {
		return "", "", false
	}
	if ok {
		return name, value, true
	}
	return name, "", true
}

func take(argv []string, allowed ...string) ([]string, map[string][]string, error) {
	positional := []string{}
	values := map[string][]string{}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			return nil, nil, usageFailure{"unexpected --"}
		}
		name, value, ok := flagParts(arg)
		if !ok {
			if strings.HasPrefix(arg, "-") {
				return nil, nil, usageFailure{fmt.Sprintf("unknown option %s", arg)}
			}
			positional = append(positional, arg)
			continue
		}
		if name == "json" {
			if value != "" {
				return nil, nil, usageFailure{"--json does not take a value"}
			}
			asJSON = true
			continue
		}
		if value == "" {
			if i+1 >= len(argv) || argv[i+1] == "--" || isFlag(argv[i+1]) {
				return nil, nil, usageFailure{fmt.Sprintf("--%s needs a value", name)}
			}
			i++
			value = argv[i]
		}
		if value == "" {
			return nil, nil, usageFailure{fmt.Sprintf("--%s needs a value", name)}
		}
		values[name] = append(values[name], value)
	}
	permitted := map[string]bool{}
	for _, name := range allowed {
		permitted[name] = true
	}
	for name := range values {
		if !permitted[name] {
			return nil, nil, usageFailure{fmt.Sprintf("unknown option --%s", name)}
		}
	}
	return positional, values, nil
}

func noExtra(positional []string) error {
	if len(positional) > 0 {
		return usageFailure{fmt.Sprintf("unexpected argument %q", positional[0])}
	}
	return nil
}

func oneArg(positional []string, what string) (string, error) {
	if len(positional) != 1 || positional[0] == "" {
		return "", usageFailure{fmt.Sprintf("a %s is required", what)}
	}
	return positional[0], nil
}

func twoArgs(positional []string, first, second string) (string, string, error) {
	if len(positional) != 2 || positional[0] == "" || positional[1] == "" {
		return "", "", usageFailure{fmt.Sprintf("a %s and a %s are required", first, second)}
	}
	return positional[0], positional[1], nil
}

func required(values map[string][]string, name string) (string, error) {
	got := values[name]
	if len(got) != 1 || got[0] == "" {
		return "", usageFailure{fmt.Sprintf("--%s is required", name)}
	}
	return got[0], nil
}

func optional(values map[string][]string, name string) (string, error) {
	got := values[name]
	switch len(got) {
	case 0:
		return "", nil
	case 1:
		if got[0] == "" {
			return "", usageFailure{fmt.Sprintf("--%s needs a value", name)}
		}
		return got[0], nil
	default:
		return "", usageFailure{fmt.Sprintf("--%s was given more than once", name)}
	}
}

func positive(value, name string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, usageFailure{fmt.Sprintf("--%s must be a positive integer", name)}
	}
	return parsed, nil
}

func writeJSON(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	cleaned := compactJSON(decoded, "")
	if cleaned == nil {
		cleaned = map[string]any{}
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cleaned)
}

func compactJSON(value any, key string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for name, item := range typed {
			cleaned := compactJSON(item, name)
			if cleaned == nil {
				continue
			}
			out[name] = cleaned
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			cleaned := compactJSON(item, "")
			if cleaned == nil {
				continue
			}
			out = append(out, cleaned)
		}
		return out
	case string:
		if typed == "" {
			return nil
		}
		return typed
	case float64:
		if typed == 0 && !keepZero(key) {
			return nil
		}
		return typed
	case bool:
		if !typed && !keepZero(key) {
			return nil
		}
		return typed
	default:
		return value
	}
}

func keepZero(key string) bool {
	switch key {
	case "count", "jobstatus", "success":
		return true
	default:
		return false
	}
}

func listOnly(argv []string, call func() (any, error)) error {
	if len(argv) == 0 {
		return usageFailure{"list is required"}
	}
	if argv[0] != "list" {
		return usageFailure{fmt.Sprintf("unknown command %q", argv[0])}
	}
	positional, _, err := take(argv[1:])
	if err != nil {
		return err
	}
	if err := noExtra(positional); err != nil {
		return err
	}
	result, err := call()
	if err != nil {
		return err
	}
	return present(result, func() error { return presentList(result) })
}
