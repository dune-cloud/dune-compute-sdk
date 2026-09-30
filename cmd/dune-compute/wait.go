// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dune-cloud/dune-compute-sdk/dune"
)

// action is the status line for one state change. target is the name or ID
// the user typed. When it is empty, the finished line uses the result name or ID.
type action struct {
	doing  string
	done   string
	target string
}

func (a action) progressLine(pct int) string {
	return fmt.Sprintf("%s ... %d%%", a.text(a.doing, ""), pct)
}

func (a action) doneLine(value any) string {
	return a.text(a.done, labelOf(value))
}

func (a action) text(lead, fallback string) string {
	target := a.target
	if target == "" {
		target = fallback
	}
	lead = strings.TrimSpace(lead)
	if target == "" {
		return lead
	}
	return lead + " " + target
}

func finish(client *dune.Client, value any, act action) error {
	settled, err := settle(client, value, act)
	if err != nil {
		return err
	}
	return writeResult(act, settled)
}

func settle(client *dune.Client, value any, act action) (any, error) {
	if err := failedJob(value); err != nil {
		return nil, err
	}
	jobID, pending := pendingJobID(value)
	if !pending {
		return value, nil
	}
	result, err := waitJob(client, jobID, jobTimeout, act)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return value, nil
	}
	return result, nil
}

func pendingJobID(value any) (string, bool) {
	object, err := objectOf(value)
	if err != nil {
		return "", false
	}
	jobID, _ := object["jobid"].(string)
	if jobID == "" {
		return "", false
	}
	status, ok := object["jobstatus"].(float64)
	if !ok || status != 0 {
		return "", false
	}
	return jobID, true
}

func failedJob(value any) error {
	object, err := objectOf(value)
	if err != nil {
		return nil
	}
	status, _ := object["jobstatus"].(float64)
	if status != 2 {
		return nil
	}
	jobID, _ := object["jobid"].(string)
	if jobID == "" {
		return fmt.Errorf("job failed")
	}
	return fmt.Errorf("job %s failed", jobID)
}

func waitJob(client *dune.Client, jobID string, timeout time.Duration, act action) (any, error) {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			clearProgress()
			return nil, fmt.Errorf("timed out waiting for job %s", jobID)
		}
		result, err := client.Job.Query(client.Job.NewQueryParams(jobID))
		if err != nil {
			clearProgress()
			return nil, err
		}
		switch result.Jobstatus {
		case 1:
			return unwrapResult(result.Jobresult), nil
		case 2:
			clearProgress()
			return nil, jobFailure(result)
		}
		showProgress(act.progressLine(result.Jobprocstatus))
		remaining := time.Until(deadline)
		wait := jobPollDelay
		if remaining < wait {
			wait = remaining
		}
		if wait > 0 {
			time.Sleep(wait)
		}
	}
}

func jobFailure(result *dune.QueryJobResponse) error {
	if result.Jobresulttype == "text" {
		text := strings.Trim(strings.TrimSpace(string(result.Jobresult)), `"`)
		if text != "" && text != "null" {
			return fmt.Errorf("job %s failed: %s", result.JobID, text)
		}
	}
	var body map[string]any
	if json.Unmarshal(result.Jobresult, &body) == nil {
		if text, ok := body["errortext"].(string); ok && text != "" {
			return fmt.Errorf("job %s failed: %s", result.JobID, text)
		}
	}
	text := strings.TrimSpace(string(result.Jobresult))
	if text != "" && text != "null" && len(text) <= 200 {
		return fmt.Errorf("job %s failed: %s", result.JobID, text)
	}
	return fmt.Errorf("job %s failed", result.JobID)
}

func unwrapResult(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
		if text == "" {
			return nil
		}
		return map[string]any{"displaytext": text}
	}
	return unwrapValue(decoded)
}

func unwrapValue(value any) any {
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if resourceLike(object) {
		return object
	}
	if len(object) == 1 {
		for _, child := range object {
			return unwrapValue(child)
		}
	}
	return object
}

func showProgress(message string) {
	if message == progressShown {
		return
	}
	progressShown = message
	width := utf8.RuneCountInString(message)
	if width > progressWidth {
		progressWidth = width
	}
	if stderrIsTTY() {
		fmt.Fprintf(stderr, "\r%s%s", message, strings.Repeat(" ", progressWidth-width))
		return
	}
	fmt.Fprintln(stderr, message)
}

// finishProgress replaces the in-progress status line with the final sentence.
// A terminal sees that one line updated in place. A pipe keeps the sentence on stdout.
func finishProgress(final string) error {
	width := progressWidth
	progressShown = ""
	progressWidth = 0
	if stderrIsTTY() && width > 0 {
		pad := width - utf8.RuneCountInString(final)
		if pad < 0 {
			pad = 0
		}
		if _, err := fmt.Fprintf(stderr, "\r%s%s\n", final, strings.Repeat(" ", pad)); err != nil {
			return err
		}
		if stdoutIsTTY() {
			return nil
		}
	}
	_, err := fmt.Fprintln(stdout, final)
	return err
}

func clearProgress() {
	width := progressWidth
	progressShown = ""
	progressWidth = 0
	if !stderrIsTTY() || width == 0 {
		return
	}
	fmt.Fprintf(stderr, "\r%s\r", strings.Repeat(" ", width))
}

func stderrIsTTY() bool {
	return writerIsTTY(stderr)
}

func stdoutIsTTY() bool {
	return writerIsTTY(stdout)
}

func writerIsTTY(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
