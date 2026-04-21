package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

// framesForRate returns a buffer size that produces ~100ms chunks at the given sample rate.
func framesForRate(sampleRate int) int {
	return sampleRate / 10 // 100ms worth of frames
}

type AudioCapture struct {
	stream      *portaudio.Stream
	buf         []int16
	raw         []byte
	writer      io.Writer
	stopCh      chan struct{}
	doneCh      chan struct{}
	once        sync.Once
	numChannels int
	channel     int // 0-indexed channel to extract
	fpb         int // frames per buffer
}

// ListInputDevices returns all available audio input devices.
func ListInputDevices() ([]*portaudio.DeviceInfo, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, err
	}
	var inputs []*portaudio.DeviceInfo
	for _, d := range devices {
		if d.MaxInputChannels > 0 {
			inputs = append(inputs, d)
		}
	}
	return inputs, nil
}

// FindDevice looks up an input device by name. Returns nil if not found.
func FindDevice(name string) (*portaudio.DeviceInfo, error) {
	devices, err := ListInputDevices()
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		if d.Name == name {
			return d, nil
		}
	}
	return nil, fmt.Errorf("audio device %q not found — run ptt --devices to list available devices", name)
}

// NewAudioCapture opens a mic at the given sample rate.
// If deviceName is empty, the default input device is used.
// channel is 1-indexed (0 = default/first channel).
// Audio is written to w as raw PCM16 little-endian bytes.
// Returns the capture and the actual sample rate used.
func NewAudioCapture(sampleRate int, deviceName string, channel int, w io.Writer) (*AudioCapture, int, error) {
	var stream *portaudio.Stream
	var err error
	numChannels := 1
	chIdx := 0 // 0-indexed

	fpb := framesForRate(sampleRate)

	if deviceName == "" {
		buf := make([]int16, fpb)
		stream, err = portaudio.OpenDefaultStream(1, 0, float64(sampleRate), fpb, buf)
		if err != nil {
			return nil, 0, err
		}
		return &AudioCapture{
			stream:      stream,
			buf:         buf,
			raw:         make([]byte, fpb*2),
			writer:      w,
			stopCh:      make(chan struct{}),
			doneCh:      make(chan struct{}),
			numChannels: 1,
			channel:     0,
			fpb:         fpb,
		}, sampleRate, nil
	}

	dev, findErr := FindDevice(deviceName)
	if findErr != nil {
		return nil, 0, findErr
	}

	// Use device's default sample rate for best compatibility
	actualRate := int(dev.DefaultSampleRate)
	if actualRate <= 0 {
		actualRate = sampleRate
	}
	fpb = framesForRate(actualRate)

	numChannels = dev.MaxInputChannels
	if channel > 0 {
		chIdx = channel - 1 // convert 1-indexed to 0-indexed
	}
	if chIdx >= numChannels {
		return nil, 0, fmt.Errorf("channel %d out of range — device has %d channels", channel, numChannels)
	}

	log.Printf("[audio] device %q: rate=%dHz, channels=%d, using ch%d", deviceName, actualRate, numChannels, chIdx+1)

	buf := make([]int16, fpb*numChannels)
	params := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   dev,
			Channels: numChannels,
			Latency:  dev.DefaultLowInputLatency,
		},
		SampleRate:      float64(actualRate),
		FramesPerBuffer: fpb,
	}
	stream, err = portaudio.OpenStream(params, buf)
	if err != nil {
		return nil, 0, err
	}
	return &AudioCapture{
		stream:      stream,
		buf:         buf,
		raw:         make([]byte, fpb*2),
		writer:      w,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		numChannels: numChannels,
		channel:     chIdx,
		fpb:         fpb,
	}, actualRate, nil
}

// SetWriter sets the audio output destination. Must be called before Start.
func (a *AudioCapture) SetWriter(w io.Writer) {
	a.writer = w
}

// Start begins capturing audio in a goroutine. Returns immediately.
func (a *AudioCapture) Start() error {
	if err := a.stream.Start(); err != nil {
		return err
	}
	go a.loop()
	return nil
}

func (a *AudioCapture) loop() {
	// The loop goroutine owns the stream lifecycle.
	// Stop/Close must only happen here — never from another goroutine.
	defer close(a.doneCh)
	defer a.stream.Close()
	for {
		select {
		case <-a.stopCh:
			return
		default:
		}
		if err := a.stream.Read(); err != nil {
			select {
			case <-a.stopCh:
				return
			default:
			}
			if err.Error() == "Input overflowed" {
				continue
			}
			log.Printf("[audio] read error: %v", err)
			return
		}
		if a.writer != nil {
			for i := 0; i < a.fpb; i++ {
				s := a.buf[i*a.numChannels+a.channel]
				binary.LittleEndian.PutUint16(a.raw[i*2:], uint16(s))
			}
			if _, err := a.writer.Write(a.raw); err != nil {
				log.Printf("[audio] write error: %v", err)
				return
			}
		}
	}
}

// TestMic captures audio for 3 seconds and prints levels per channel.
func TestMic(sampleRate int, deviceName string) {
	dev := "default"
	if deviceName != "" {
		dev = deviceName
	}

	// Figure out how many input channels the device has
	numChannels := 1
	if deviceName != "" {
		d, findErr := FindDevice(deviceName)
		if findErr != nil {
			log.Fatalf("[test-mic] %v", findErr)
		}
		if d.MaxInputChannels > 1 {
			numChannels = d.MaxInputChannels
		}
	}

	fpb := framesForRate(sampleRate)
	fmt.Printf("Testing mic: %s (sample rate: %d, channels: %d)\n", dev, sampleRate, numChannels)
	fmt.Println("Speak into your mic for 3 seconds...")

	buf := make([]int16, fpb*numChannels)
	var stream *portaudio.Stream
	var err error

	if deviceName == "" {
		stream, err = portaudio.OpenDefaultStream(numChannels, 0, float64(sampleRate), fpb, buf)
	} else {
		d, _ := FindDevice(deviceName)
		params := portaudio.StreamParameters{
			Input: portaudio.StreamDeviceParameters{
				Device:   d,
				Channels: numChannels,
				Latency:  d.DefaultLowInputLatency,
			},
			SampleRate:      float64(sampleRate),
			FramesPerBuffer: fpb,
		}
		stream, err = portaudio.OpenStream(params, buf)
	}
	if err != nil {
		log.Fatalf("[test-mic] open stream: %v", err)
	}
	defer stream.Close()

	if err := stream.Start(); err != nil {
		log.Fatalf("[test-mic] start: %v", err)
	}
	defer stream.Stop()

	iterations := (sampleRate * 3) / fpb
	maxPeaks := make([]int16, numChannels)

	for i := 0; i < iterations; i++ {
		if err := stream.Read(); err != nil {
			log.Fatalf("[test-mic] read: %v", err)
		}

		peaks := make([]int16, numChannels)
		for j := 0; j < fpb; j++ {
			for ch := 0; ch < numChannels; ch++ {
				s := buf[j*numChannels+ch]
				if s < 0 {
					s = -s
				}
				if s > peaks[ch] {
					peaks[ch] = s
				}
			}
		}
		for ch := 0; ch < numChannels; ch++ {
			if peaks[ch] > maxPeaks[ch] {
				maxPeaks[ch] = peaks[ch]
			}
		}

		if i%10 == 0 {
			fmt.Print("\r")
			for ch := 0; ch < numChannels; ch++ {
				bars := int(peaks[ch]) * 30 / 32768
				meter := ""
				for j := 0; j < bars; j++ {
					meter += "#"
				}
				fmt.Printf("  ch%d: %-30s %5d  ", ch+1, meter, peaks[ch])
			}
		}
	}

	fmt.Println()
	for ch := 0; ch < numChannels; ch++ {
		status := "NO SIGNAL"
		if maxPeaks[ch] >= 1000 {
			status = "OK"
		} else if maxPeaks[ch] >= 100 {
			status = "VERY QUIET"
		}
		fmt.Printf("\n  Channel %d — peak: %d / 32768 — %s", ch+1, maxPeaks[ch], status)
	}
	fmt.Println()
}

// Stop signals the audio loop to exit and waits for it.
// The loop goroutine handles all stream cleanup.
func (a *AudioCapture) Stop() {
	a.once.Do(func() {
		close(a.stopCh)
		// Loop will see stopCh after current Read() returns (~100ms max).
		// Safety timeout in case Read() blocks indefinitely on some devices.
		select {
		case <-a.doneCh:
		case <-time.After(2 * time.Second):
			log.Println("[audio] warning: audio loop did not exit in time, leaking")
		}
	})
}
