package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendTelegramWithURL(t *testing.T) {
	var gotChatID, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botsecret-token/sendMessage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotChatID = body["chat_id"]
		gotText = body["text"]
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	err := SendTelegramWithURL(srv.URL, "secret-token", "123456", "Portfolio synced")
	if err != nil {
		t.Fatalf("SendTelegramWithURL failed: %v", err)
	}
	if gotChatID != "123456" || gotText != "Portfolio synced" {
		t.Errorf("unexpected payload: chat_id=%s text=%s", gotChatID, gotText)
	}
}
