package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginStart(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/voice/login/start" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"device_code":"ABCD1234","verification_url":"https://ologi.app/voice/link?code=ABCD1234","interval_ms":2000}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "") // no API key for /start
	c.Version = "0.0.1-test"

	out, err := c.LoginStart("brent-mbp")
	if err != nil {
		t.Fatalf("LoginStart: %v", err)
	}
	if out.DeviceCode != "ABCD1234" {
		t.Errorf("device_code: got %q", out.DeviceCode)
	}
	if out.IntervalMs != 2000 {
		t.Errorf("interval_ms: got %d", out.IntervalMs)
	}
	if gotBody["device_name"] != "brent-mbp" || gotBody["platform"] != "darwin" || gotBody["cli_version"] != "0.0.1-test" {
		t.Errorf("body: %+v", gotBody)
	}
}

func TestLoginPoll_Pending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/voice/login/complete" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"status":"pending"}`))
	}))
	defer srv.Close()

	out, err := NewClient(srv.URL, "").LoginPoll("ABCD1234")
	if err != nil {
		t.Fatalf("LoginPoll: %v", err)
	}
	if out.Status != "pending" {
		t.Errorf("status: got %q, want pending", out.Status)
	}
}

func TestLoginPoll_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok","api_key":"ht_real","device_id":"dev-1"}`))
	}))
	defer srv.Close()

	out, err := NewClient(srv.URL, "").LoginPoll("ABCD1234")
	if err != nil {
		t.Fatalf("LoginPoll: %v", err)
	}
	if out.Status != "ok" {
		t.Errorf("status: got %q", out.Status)
	}
	if out.APIKey != "ht_real" || out.DeviceID != "dev-1" {
		t.Errorf("fields: %+v", out)
	}
}

func TestLoginPoll_Expired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(410)
		w.Write([]byte(`{"status":"expired"}`))
	}))
	defer srv.Close()

	out, err := NewClient(srv.URL, "").LoginPoll("ABCD1234")
	if err != nil {
		t.Fatalf("LoginPoll (want nil err, 410 is an expected terminal status): %v", err)
	}
	if out.Status != "expired" {
		t.Errorf("status: got %q, want expired", out.Status)
	}
}

func TestDeleteDevice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/voice/devices/dev-1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "ht_test").DeleteDevice("dev-1"); err != nil {
		t.Fatalf("DeleteDevice: %v", err)
	}
}
