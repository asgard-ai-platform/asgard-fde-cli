package platform

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The id must be the Console's, byte for byte: a different one is a second
// channel and a second sandbox. The expected value is SHA-256 of the scope,
// computed outside Go.
func TestPreviewChannelIDMatchesTheConsole(t *testing.T) {
	if got := PreviewChannelID(AgentPreviewScope("a", "u")); got != "preview_94c8376f0710ae6bb037149751db3aa3" {
		t.Errorf("got %s", got)
	}
	if got := len(PreviewChannelID(FlowPreviewScope("u"))); got != 40 {
		t.Errorf("a preview id is 40 characters; got %d", got)
	}
}

func TestReadFrames(t *testing.T) {
	stream := ": keep-alive\n\n" +
		"id: 1\nevent: asgard.run.init\ndata: {\"eventType\":\"asgard.run.init\"}\n\n" +
		"id: 2\ndata: {\"eventType\":\"asgard.message.delta\",\n" +
		"data: \"fact\":{}}\n\n" +
		"id: 3\nevent: asgard.run.done\ndata: {}\n\n" +
		"id: 4\nevent: asgard.message.delta\ndata: {}\n\n"
	var types []string
	done, cursor, err := readFrames(strings.NewReader(stream), func(ev ChatEvent) error {
		types = append(types, ev.Type)
		return nil
	})
	if err != nil || !done || cursor != "3" {
		t.Fatalf("done %v cursor %q err %v", done, cursor, err)
	}
	// The second frame has no event line and is typed from its data, which
	// was split over two data lines.
	if strings.Join(types, ",") != "asgard.run.init,asgard.message.delta,asgard.run.done" {
		t.Errorf("types %v", types)
	}
}

// A stream cut after a cursor is rejoined from that cursor, and the turn is
// never sent a second time.
func TestSendChatResumesByRejoiningNeverBySendingAgain(t *testing.T) {
	var lastEventID string
	srv, seen := fakePlatform(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, "id: 7\nevent: asgard.message.delta\ndata: {}\n\n")
			return
		}
		lastEventID = r.Header.Get("Last-Event-ID")
		_, _ = io.WriteString(w, "id: 8\nevent: asgard.run.done\ndata: {}\n\n")
	})
	resumeBackoff = []time.Duration{time.Millisecond}
	defer func() { resumeBackoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second} }()

	var types []string
	err := sandboxClient(srv.URL).SendChat(context.Background(), "p-1", TriggerChat("t"), ChatMessage{CustomChannelID: "inv"},
		func(ev ChatEvent) error { types = append(types, ev.Type); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 2 || (*seen)[0].method != http.MethodPost || (*seen)[1].method != http.MethodGet || lastEventID != "7" {
		t.Errorf("calls %+v, Last-Event-ID %q", *seen, lastEventID)
	}
	if strings.Join(types, ",") != "asgard.message.delta,asgard.run.done" {
		t.Errorf("types %v", types)
	}
}

// With no cursor there is nothing to rejoin from, and sending again could
// start the turn twice, so the stream ending is the answer.
func TestSendChatWithoutACursorDoesNotRetry(t *testing.T) {
	srv, seen := fakePlatform(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
	})
	err := sandboxClient(srv.URL).SendChat(context.Background(), "p-1", TriggerChat("t"), ChatMessage{CustomChannelID: "inv"},
		func(ChatEvent) error { return nil })
	if !errors.Is(err, ErrStreamEnded) || len(*seen) != 1 {
		t.Fatalf("err %v after %d calls", err, len(*seen))
	}
}
