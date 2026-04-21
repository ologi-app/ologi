package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/ologi/hypertask-cli/internal/api"
)

type fakeAPI struct {
	config    api.ConfigResponse
	configErr error
	mintErr   error
	token     string
	postErr   error
	lastPost  api.PostSessionInput
}

func (f *fakeAPI) GetConfig() (api.ConfigResponse, error) {
	return f.config, f.configErr
}
func (f *fakeAPI) MintRealtimeToken() (string, error) { return f.token, f.mintErr }
func (f *fakeAPI) PostSession(in api.PostSessionInput) (api.PostSessionResponse, error) {
	f.lastPost = in
	return api.PostSessionResponse{SessionID: "s1", CanonicalText: in.Text, SettingsVersion: f.config.SettingsVersion}, f.postErr
}

func TestRuntimeBootReturnsEngineConfig(t *testing.T) {
	device := "MacBook Pro Mic"
	fake := &fakeAPI{config: api.ConfigResponse{
		SettingsVersion: 2,
		Hotkey:          "right_option",
		Language:        "en",
		MicDevice:       &device,
		StartSound:      "Tink",
		StopSound:       "Pop",
	}}
	rt := &Runtime{Client: fake}

	cfg, err := rt.Boot()
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if cfg.Hotkey != "right_option" || cfg.Language != "en" || cfg.Device != device {
		t.Errorf("unexpected cfg: %+v", cfg)
	}
	if cfg.SampleRate != 16000 || cfg.Mode != "stream" {
		t.Errorf("wrong defaults: sr=%d mode=%q", cfg.SampleRate, cfg.Mode)
	}
}

func TestRuntimeBootSurfacesAuthError(t *testing.T) {
	rt := &Runtime{Client: &fakeAPI{configErr: errors.New("401")}}
	if _, err := rt.Boot(); err == nil {
		t.Error("want error, got nil")
	}
}

func TestRuntimeOnSessionPostsToAPI(t *testing.T) {
	fake := &fakeAPI{token: "tok"}
	rt := &Runtime{
		Client: fake,
		Detect: func() string { return "iTerm2" },
	}

	start := time.Now()
	end := start.Add(time.Second)
	rt.OnSession(Session{
		Mode:       "stream",
		StartedAt:  start,
		EndedAt:    end,
		DurationMs: 1000,
		Text:       "hello",
	})

	if fake.lastPost.Mode != "stream" || fake.lastPost.Text != "hello" {
		t.Errorf("unexpected post: %+v", fake.lastPost)
	}
	if fake.lastPost.SourceApp == nil || *fake.lastPost.SourceApp != "iTerm2" {
		t.Errorf("source_app: got %v", fake.lastPost.SourceApp)
	}
}
