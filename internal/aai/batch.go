package aai

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	assemblyAIUploadURL    = "https://api.assemblyai.com/v2/upload"
	assemblyAITranscriptURL = "https://api.assemblyai.com/v2/transcript"
)

// BatchRecorder accumulates audio in memory for batch transcription.
type BatchRecorder struct {
	mu         sync.Mutex
	buf        bytes.Buffer
	apiKey     string
	language   string
	sampleRate int
}

func NewBatchRecorder(apiKey, language string, sampleRate int) *BatchRecorder {
	return &BatchRecorder{
		apiKey:     apiKey,
		language:   language,
		sampleRate: sampleRate,
	}
}

// Write accumulates PCM16 audio data. Implements io.Writer.
func (b *BatchRecorder) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// Size returns the current buffer size in bytes.
func (b *BatchRecorder) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// Transcribe uploads the recorded audio and returns the transcribed text.
// Blocks until transcription is complete.
func (b *BatchRecorder) Transcribe() (string, error) {
	b.mu.Lock()
	pcmData := make([]byte, b.buf.Len())
	copy(pcmData, b.buf.Bytes())
	b.buf.Reset()
	b.mu.Unlock()

	if len(pcmData) == 0 {
		return "", nil
	}

	// Wrap PCM16 data in a WAV container
	wavData := pcmToWAV(pcmData, b.sampleRate)

	// Upload audio
	uploadURL, err := b.upload(wavData)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}

	// Create transcript
	transcriptID, err := b.createTranscript(uploadURL)
	if err != nil {
		return "", fmt.Errorf("create transcript: %w", err)
	}

	// Poll for completion
	text, err := b.pollTranscript(transcriptID)
	if err != nil {
		return "", fmt.Errorf("poll: %w", err)
	}

	return text, nil
}

func (b *BatchRecorder) upload(data []byte) (string, error) {
	req, err := http.NewRequest("POST", assemblyAIUploadURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", b.apiKey)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, body)
	}

	var result struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.UploadURL, nil
}

func (b *BatchRecorder) createTranscript(audioURL string) (string, error) {
	body := map[string]interface{}{
		"audio_url":     audioURL,
		"speech_models": []string{"universal-3-pro"},
	}
	if b.language != "" && b.language != "en" {
		body["language_code"] = b.language
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", assemblyAITranscriptURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", b.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create transcript failed (status %d): %s", resp.StatusCode, respBody)
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Status == "error" {
		return "", fmt.Errorf("transcript error: %s", result.Error)
	}
	return result.ID, nil
}

func (b *BatchRecorder) pollTranscript(id string) (string, error) {
	url := assemblyAITranscriptURL + "/" + id

	for {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", b.apiKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}

		var result struct {
			Status string `json:"status"`
			Text   string `json:"text"`
			Error  string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return "", err
		}
		resp.Body.Close()

		switch result.Status {
		case "completed":
			return result.Text, nil
		case "error":
			return "", fmt.Errorf("transcription failed: %s", result.Error)
		default:
			log.Printf("[batch] status: %s, waiting...", result.Status)
			time.Sleep(1 * time.Second)
		}
	}
}

// pcmToWAV wraps raw PCM16 mono data in a WAV header.
func pcmToWAV(pcm []byte, sampleRate int) []byte {
	var buf bytes.Buffer
	numChannels := 1
	bitsPerSample := 16
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := len(pcm)
	fileSize := 36 + dataSize

	// RIFF header
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, int32(fileSize))
	buf.WriteString("WAVE")

	// fmt subchunk
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, int32(16))           // subchunk size
	binary.Write(&buf, binary.LittleEndian, int16(1))            // PCM format
	binary.Write(&buf, binary.LittleEndian, int16(numChannels))  // channels
	binary.Write(&buf, binary.LittleEndian, int32(sampleRate))   // sample rate
	binary.Write(&buf, binary.LittleEndian, int32(byteRate))     // byte rate
	binary.Write(&buf, binary.LittleEndian, int16(blockAlign))   // block align
	binary.Write(&buf, binary.LittleEndian, int16(bitsPerSample)) // bits per sample

	// data subchunk
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, int32(dataSize))
	buf.Write(pcm)

	return buf.Bytes()
}
