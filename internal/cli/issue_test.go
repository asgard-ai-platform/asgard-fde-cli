package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/feedback"
)

// The discovery section closes every report, so a defect report can carry a
// discovery without a second shape.
func TestIssueReportCarriesADiscoverySection(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	if err := writeReport(&out); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if !strings.Contains(body, "## 6) What I now know") {
		t.Fatalf("no discovery section:\n%s", body)
	}
	if strings.Index(body, "## 6)") < strings.Index(body, "## 5)") {
		t.Errorf("section 6 should come after section 5:\n%s", body)
	}
	// Section 6 is optional, so its placeholder must not be a marker --send
	// refuses: a defect report that leaves it as written is complete.
	for _, s := range unfilled(body) {
		if s == "6" {
			t.Errorf("section 6's placeholder is treated as unfilled:\n%s", body)
		}
	}
}

// TestIssueReportSend walks the route the help gives: --new, fill it in,
// --send. What --new writes is refused as it stands, and the same report with
// its TODOs answered reaches Sentry.
func TestIssueReportSend(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received = string(b)
	}))
	defer srv.Close()
	old := feedback.DSN
	feedback.DSN = strings.Replace(srv.URL, "http://", "http://k@", 1) + "/1"
	defer func() { feedback.DSN = old }()

	draft, _, err := runCLI(t, "", "issue-report", "--new")
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = runCLI(t, draft, "issue-report", "--send", "-")
	if err == nil || !strings.Contains(err.Error(), "section 1, 3, 4, 5:") {
		t.Fatalf("an unfilled report has to be refused, naming its sections: %v", err)
	}
	if received != "" {
		t.Fatal("a refused report reached Sentry")
	}

	filled := strings.ReplaceAll(draft, "TODO - ", "")
	out, stderr, err := runCLI(t, filled, "issue-report", "--send", "-", "--email", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Sent.") || !strings.Contains(stderr, "Sentry User Feedback") {
		t.Errorf("stdout %q, stderr %q", out, stderr)
	}
	if !strings.Contains(received, `"type":"feedback"`) || !strings.Contains(received, "## 2) The state I was in") {
		t.Errorf("Sentry received:\n%s", received)
	}
}

func TestIssueReportNewAndSendExclusive(t *testing.T) {
	if _, _, err := runCLI(t, "", "issue-report", "--new", "--send", "-"); err == nil {
		t.Error("--new with --send has to be refused")
	}
}
