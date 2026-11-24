package mimir

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
)

// sseEvent is a single Server-Sent Event as delimited by a blank line.
type sseEvent struct {
	Event string
	ID    string
	Data  []byte
	Done  bool // "data: [DONE]" terminal marker
}

// parseSSE reads a text/event-stream from r and invokes onEvent per event.
// It returns (true, nil) when the terminal [DONE] marker is seen and the stream
// ends cleanly. If onEvent returns an error, parsing stops and that error is
// returned (with sawDone=false). Context cancellation yields ErrCanceled.
//
// It is tolerant of CRLF line endings, comment lines (":..."), and multiple
// data lines per event (joined with "\n").
func parseSSE(ctx context.Context, r io.Reader, onEvent func(sseEvent) error) (bool, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	var ev sseEvent
	var dataParts []string
	sawData := false

	flush := func() error {
		if !sawData {
			ev = sseEvent{}
			dataParts = nil
			return nil
		}
		joined := strings.Join(dataParts, "\n")
		out := sseEvent{Event: ev.Event, ID: ev.ID}
		if joined == "[DONE]" {
			out.Done = true
		} else {
			out.Data = []byte(joined)
		}
		ev = sseEvent{}
		dataParts = nil
		sawData = false
		return onEvent(out)
	}

	for {
		if err := ctx.Err(); err != nil {
			return false, ErrCanceled
		}
		line, err := readLine(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Flush any trailing event without a final blank line.
				if ferr := flush(); ferr != nil {
					return false, ferr
				}
				return true, nil // clean end of stream
			}
			return false, err
		}
		line = strings.TrimRight(line, "\r")

		if line == "" {
			if ferr := flush(); ferr != nil {
				return false, ferr
			}
			continue
		}
		if strings.HasPrefix(line, ":") { // comment / keep-alive
			continue
		}

		field, value := splitField(line)
		switch field {
		case "data":
			value = strings.TrimPrefix(value, " ")
			dataParts = append(dataParts, value)
			sawData = true
		case "event":
			ev.Event = strings.TrimPrefix(value, " ")
		case "id":
			ev.ID = strings.TrimPrefix(value, " ")
		case "retry":
			// ignored
		}
	}
}

// readLine reads a single LF-terminated line without a token size limit.
func readLine(br *bufio.Reader) (string, error) {
	var sb strings.Builder
	for {
		part, err := br.ReadString('\n')
		sb.WriteString(part)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Return any partial line, then EOF is surfaced next call.
				if sb.Len() == 0 {
					return "", io.EOF
				}
				return strings.TrimSuffix(sb.String(), "\n"), io.EOF
			}
			return "", err
		}
		return strings.TrimSuffix(sb.String(), "\n"), nil
	}
}

func splitField(line string) (field, value string) {
	if i := strings.IndexByte(line, ':'); i >= 0 {
		return line[:i], line[i+1:]
	}
	return line, ""
}
