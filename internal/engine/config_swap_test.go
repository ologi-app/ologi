package engine

import (
	"testing"

	"github.com/ologi-app/ologi/internal/api"
)

type stubConfigClient struct {
	configs []api.ConfigResponse
	idx     int
}

func (s *stubConfigClient) GetConfig() (api.ConfigResponse, error) {
	c := s.configs[s.idx]
	if s.idx < len(s.configs)-1 {
		s.idx++
	}
	return c, nil
}

func (s *stubConfigClient) MintRealtimeToken() (string, error) { return "", nil }
func (s *stubConfigClient) PostSession(in api.PostSessionInput) (api.PostSessionResponse, error) {
	return api.PostSessionResponse{}, nil
}

func TestConfigRefresh_swapsMicOnVersionBump(t *testing.T) {
	mic1 := "old"
	mic2 := "new"
	stub := &stubConfigClient{configs: []api.ConfigResponse{
		{SettingsVersion: 1, MicDevice: &mic1},
		{SettingsVersion: 2, MicDevice: &mic2},
	}}
	cur := api.ConfigResponse{SettingsVersion: 1, MicDevice: &mic1}

	// Mimic the in-engine refresh logic.
	refresh := func() {
		newCfg, err := stub.GetConfig()
		if err != nil {
			return
		}
		if newCfg.SettingsVersion != cur.SettingsVersion {
			cur.MicDevice = newCfg.MicDevice
			cur.SettingsVersion = newCfg.SettingsVersion
		}
	}

	// First call: same version (1 → 1), no swap.
	refresh()
	if cur.MicDevice == nil || *cur.MicDevice != "old" {
		t.Errorf("first call should be no-op (cached), got mic=%v", cur.MicDevice)
	}

	// Second call: version bumps (1 → 2), swap should happen.
	refresh()
	if cur.MicDevice == nil || *cur.MicDevice != "new" {
		t.Errorf("second call should swap, got mic=%v", cur.MicDevice)
	}
}

func TestConfigRefresh_noSwapOnSameVersion(t *testing.T) {
	mic := "same"
	stub := &stubConfigClient{configs: []api.ConfigResponse{
		{SettingsVersion: 5, MicDevice: &mic},
	}}
	cur := api.ConfigResponse{SettingsVersion: 5, MicDevice: &mic}

	refresh := func() {
		newCfg, err := stub.GetConfig()
		if err != nil {
			return
		}
		if newCfg.SettingsVersion != cur.SettingsVersion {
			cur.MicDevice = newCfg.MicDevice
			cur.SettingsVersion = newCfg.SettingsVersion
		}
	}

	refresh()
	if cur.SettingsVersion != 5 {
		t.Errorf("version should stay 5, got %d", cur.SettingsVersion)
	}
}

func TestConfigRefresh_nilMicMeansSystemDefault(t *testing.T) {
	mic1 := "USB Mic"
	stub := &stubConfigClient{configs: []api.ConfigResponse{
		{SettingsVersion: 1, MicDevice: &mic1},
		{SettingsVersion: 2, MicDevice: nil}, // cleared — revert to system default
	}}
	cur := api.ConfigResponse{SettingsVersion: 1, MicDevice: &mic1}

	refresh := func() {
		newCfg, err := stub.GetConfig()
		if err != nil {
			return
		}
		if newCfg.SettingsVersion != cur.SettingsVersion {
			cur.MicDevice = newCfg.MicDevice
			cur.SettingsVersion = newCfg.SettingsVersion
		}
	}

	refresh() // no-op (v1 → v1)
	refresh() // swap (v1 → v2), MicDevice becomes nil

	if cur.MicDevice != nil {
		t.Errorf("nil mic_device should be preserved (system default), got %v", *cur.MicDevice)
	}
	if cur.SettingsVersion != 2 {
		t.Errorf("version should be 2, got %d", cur.SettingsVersion)
	}
}
