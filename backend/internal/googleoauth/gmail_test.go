package googleoauth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// mockGmail serves the two Gmail endpoints the client uses, and asserts the
// bearer token is presented.
func mockGmail(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-access-token" {
			t.Errorf("Authorization = %q, want bearer test-access-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/users/me/messages/"):
			id := strings.TrimPrefix(r.URL.Path, "/users/me/messages/")
			_, _ = w.Write([]byte(`{
				"id":"` + id + `",
				"snippet":"Hello from message ` + id + `",
				"internalDate":"1700000000000",
				"payload":{"headers":[
					{"name":"From","value":"alice@example.com"},
					{"name":"Subject","value":"Subject of ` + id + `"},
					{"name":"Date","value":"Tue, 14 Nov 2023 22:13:20 +0000"}
				]}
			}`))
		case strings.HasSuffix(r.URL.Path, "/users/me/messages"):
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1"},{"id":"m2"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestGmailHistoryUsesNativeCursorAndDeduplicatesMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/users/me/history" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("startHistoryId") != "100" || r.URL.Query().Get("historyTypes") != "messageAdded" {
			t.Errorf("history query = %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"historyId":"105","history":[{"messagesAdded":[{"message":{"id":"m1"}},{"message":{"id":"m1"}},{"message":{"id":"m2"}}]}]}`))
	}))
	defer server.Close()

	page, err := (GmailClient{AccessToken: "token", BaseURL: server.URL}).ListHistoryPage(context.Background(), "100", "", 50)
	if err != nil || page.HistoryID != "105" || len(page.MessageIDs) != 2 || page.MessageIDs[0] != "m1" || page.MessageIDs[1] != "m2" {
		t.Fatalf("ListHistoryPage = %#v, %v", page, err)
	}
}

func TestGmailExpiredHistoryRequiresFullSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404}}`))
	}))
	defer server.Close()

	_, err := (GmailClient{AccessToken: "token", BaseURL: server.URL}).ListHistoryPage(context.Background(), "stale", "", 50)
	if err != ErrHistoryCursorExpired {
		t.Fatalf("error = %v, want ErrHistoryCursorExpired", err)
	}
}

func TestGmailExtractsBodyAndBoundedTextAttachment(t *testing.T) {
	body := base64.RawURLEncoding.EncodeToString([]byte("Please prepare the evidence bundle."))
	attachment := base64.RawURLEncoding.EncodeToString([]byte("Decision: use the signed version."))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/me/messages/m1":
			_, _ = w.Write([]byte(`{"id":"m1","threadId":"t1","historyId":"101","payload":{"headers":[{"name":"From","value":"lawyer@example.com"},{"name":"To","value":"robert@example.com"},{"name":"Subject","value":"Evidence"}],"parts":[{"mimeType":"text/plain","body":{"data":"` + body + `"}},{"mimeType":"text/plain","filename":"decision.txt","body":{"attachmentId":"a1","size":33}}]}}`))
		case "/users/me/messages/m1/attachments/a1":
			_, _ = w.Write([]byte(`{"size":33,"data":"` + attachment + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	message, err := (GmailClient{AccessToken: "token", BaseURL: server.URL}).GetMessageMetadata(context.Background(), "m1")
	if err != nil || !strings.Contains(message.Body, "evidence bundle") || len(message.Attachments) != 1 || !message.Attachments[0].Fetched || !strings.Contains(message.Attachments[0].Content, "signed version") {
		t.Fatalf("message = %#v, err=%v", message, err)
	}
}

func TestGmailBoundsAttachmentRecordsAndTotalExtractedText(t *testing.T) {
	attachmentData := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", maxGmailAttachmentBytes)))
	parts := make([]gmailMessagePart, 0, maxGmailAttachmentRecords+1)
	for index := 0; index < maxGmailAttachmentRecords+1; index++ {
		part := gmailMessagePart{MimeType: "text/plain", Filename: "attachment-" + strconv.Itoa(index) + ".txt"}
		part.Body.Size = maxGmailAttachmentBytes
		part.Body.Data = attachmentData
		parts = append(parts, part)
	}
	attachments := []GmailAttachment{}
	plain, html := []string{}, []string{}
	budget := maxGmailAttachmentContentTotalBytes
	(GmailClient{}).collectPart(context.Background(), "m1", gmailMessagePart{Parts: parts}, &plain, &html, &attachments, &budget)
	if len(attachments) != maxGmailAttachmentRecords {
		t.Fatalf("attachment records = %d, want %d", len(attachments), maxGmailAttachmentRecords)
	}
	extractedBytes := 0
	for _, attachment := range attachments {
		extractedBytes += len(attachment.Content)
	}
	if extractedBytes > maxGmailAttachmentContentTotalBytes {
		t.Fatalf("extracted attachment bytes = %d, exceeds %d", extractedBytes, maxGmailAttachmentContentTotalBytes)
	}
}

func TestGmailFetchRecentParsesHeadersAndSnippet(t *testing.T) {
	srv := mockGmail(t)
	defer srv.Close()

	client := GmailClient{AccessToken: "test-access-token", BaseURL: srv.URL}
	msgs, err := client.FetchRecent(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("FetchRecent: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	first := msgs[0]
	if first.ID != "m1" {
		t.Errorf("id = %q, want m1", first.ID)
	}
	if first.From != "alice@example.com" {
		t.Errorf("from = %q", first.From)
	}
	if first.Subject != "Subject of m1" {
		t.Errorf("subject = %q", first.Subject)
	}
	if first.Snippet == "" {
		t.Error("snippet should be populated")
	}
	if first.Date.IsZero() {
		t.Error("date should be parsed from internalDate")
	}
}

func TestGmailSurfacesExpiredToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401}}`))
	}))
	defer srv.Close()

	client := GmailClient{AccessToken: "stale", BaseURL: srv.URL}
	_, err := client.ListRecentMessageIDs(context.Background(), 5, "")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected a clear 401/expired error, got %v", err)
	}
}

func TestGmailRejectsResponseOverSafetyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"payload":"` + strings.Repeat("x", maxGmailResponseBytes) + `"}`))
	}))
	defer server.Close()

	err := (GmailClient{AccessToken: "token", BaseURL: server.URL}).getJSON(context.Background(), "/users/me/profile", &struct{}{})
	if err == nil || !strings.Contains(err.Error(), "response exceeded") {
		t.Fatalf("getJSON error = %v, want explicit response safety limit", err)
	}
}

// One malformed message must not fail the whole sync.
func TestGmailSkipsUnfetchableMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/users/me/messages"):
			_, _ = w.Write([]byte(`{"messages":[{"id":"ok"},{"id":"bad"}]}`))
		case strings.HasSuffix(r.URL.Path, "/messages/ok"):
			_, _ = w.Write([]byte(`{"id":"ok","snippet":"fine","payload":{"headers":[]}}`))
		default: // "bad" returns 500
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := GmailClient{AccessToken: "t", BaseURL: srv.URL}
	msgs, err := client.FetchRecent(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("FetchRecent should not fail on one bad message: %v", err)
	}
	if len(msgs) != 1 || msgs[0].ID != "ok" {
		t.Fatalf("want only the good message, got %+v", msgs)
	}
}
