package api

type patchDeviceBody struct {
	AvailableMics []string `json:"available_mics"`
	CLIVersion    string   `json:"cli_version,omitempty"`
}

// PatchDevice uploads the host's PortAudio device list and the CLI's
// own version. Server stores them on voice_devices for the browser
// settings page's mic dropdown.
func (c *Client) PatchDevice(deviceID string, availableMics []string, cliVersion string) error {
	_, err := c.do("PATCH", "/api/voice/devices/"+deviceID, patchDeviceBody{
		AvailableMics: availableMics,
		CLIVersion:    cliVersion,
	})
	return err
}
