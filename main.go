package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultBase  = "https://api.openai.com/v1"
	DefaultModel = "whisper-1"
)

// Client is the main structure for interacting with the Whisper ASR API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// ClientOption is a function type that allows to set options for the Client.
type ClientOption func(*Client)

// WithKey sets the API key for the Client.
func WithKey(key string) ClientOption {
	return func(c *Client) {
		c.apiKey = key
	}
}

// WithBaseURL sets the base URL for the Client.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHTTPClient sets the HTTP client for the Client.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// NewClient creates a new Whisper ASR API client with the given options.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{}

	for _, opt := range opts {
		opt(c)
	}

	if c.apiKey == "" {
		c.apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if c.baseURL == "" {
		c.baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if c.httpClient == nil {
		c.httpClient = http.DefaultClient
	}

	return c
}

// URL constructs the full URL for the given relative path.
func (c *Client) URL(relPath string) string {
	if strings.Contains(relPath, "://") {
		return relPath
	}
	baseURL := c.baseURL
	if baseURL == "" {
		baseURL = DefaultBase
	}
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(relPath, "/")
}

// Segment represents a segment of transcribed text in the TranscribeResponse.
type Segment struct {
	ID               int     `json:"id"`
	Seek             int     `json:"seek"`
	Start            float64 `json:"start"`
	End              float64 `json:"end"`
	Text             string  `json:"text"`
	Tokens           []int   `json:"tokens"`
	Temperature      float64 `json:"temperature"`
	AvgLogprob       float64 `json:"avg_logprob"`
	CompressionRatio float64 `json:"compression_ratio"`
	NoSpeechProb     float64 `json:"no_speech_prob"`
	Transient        bool    `json:"transient"`
}

// TranscribeResponse represents the response from the Whisper ASR API.
type TranscribeResponse struct {
	Task     string    `json:"task"`
	Language string    `json:"language"`
	Duration float64   `json:"duration"`
	Segments []Segment `json:"segments"`
	Text     string    `json:"text"`
}

// TranscribeFile transcribes the given file using the Whisper ASR API.
func (c *Client) TranscribeFile(file string, opts ...TranscribeOption) (*TranscribeResponse, error) {
	h, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer h.Close()

	opts = append([]TranscribeOption{WithFile(filepath.Base(file))}, opts...)
	return c.Transcribe(h, opts...)
}

// TranscribeConfig is a structure that holds the configuration for the Transcribe method.
type TranscribeConfig struct {
	Model    string
	Language string
	File     string
}

// TranscribeOption is a function type that allows to set options for the Transcribe method.
type TranscribeOption func(*TranscribeConfig)

// WithModel sets the model for the Transcribe method.
func WithModel(model string) TranscribeOption {
	return func(tc *TranscribeConfig) {
		tc.Model = model
	}
}

// WithLanguage sets the language for the Transcribe method.
func WithLanguage(lang string) TranscribeOption {
	return func(tc *TranscribeConfig) {
		tc.Language = lang
	}
}

// WithFile sets the file for the Transcribe method.
func WithFile(file string) TranscribeOption {
	return func(tc *TranscribeConfig) {
		tc.File = file
	}
}

// Transcribe transcribes the given audio stream using the Whisper ASR API.
func (c *Client) Transcribe(h io.Reader, opts ...TranscribeOption) (*TranscribeResponse, error) {
	if c.apiKey == "" {
		return nil, errors.New("missing API key (set OPENAI_API_KEY in env)")
	}

	tc := &TranscribeConfig{}
	for _, opt := range opts {
		opt(tc)
	}

	if tc.Model == "" {
		tc.Model = DefaultModel
	}

	if tc.File == "" {
		return nil, errors.New("filename is not set")
	}

	b := &bytes.Buffer{}
	mp := multipart.NewWriter(b)

	f, err := mp.CreateFormField("model")
	if err != nil {
		return nil, err
	}
	f.Write([]byte(tc.Model))

	if f, err = mp.CreateFormField("response_format"); err != nil {
		return nil, err
	}
	f.Write([]byte("verbose_json"))

	fp, err := mp.CreateFormFile("file", tc.File)
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(fp, h); err != nil {
		return nil, err
	}
	mp.Close()

	url := c.URL("audio/transcriptions")
	req, err := http.NewRequest(http.MethodPost, url, b)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r io.Reader
	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		r, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer r.(*gzip.Reader).Close()
	case "deflate":
		r = flate.NewReader(resp.Body)
		defer r.(io.ReadCloser).Close()
	default:
		r = resp.Body
	}

	if resp.StatusCode != http.StatusOK {
		io.Copy(os.Stderr, r)
		return nil, fmt.Errorf("unexpected response: %s", resp.Status)
	}

	var tr TranscribeResponse
	if err = json.NewDecoder(r).Decode(&tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

func main() {
	
	// by default, the key is read from OPENAI_API_KEY in env
	client := NewClient(WithKey("<your API key>"))

	response, err := client.TranscribeFile("file.m4a")
	if err != nil {
		log.Fatalf("Error transcribing file: %v", err)
	}

	fmt.Printf("Transcription: %s\n", response.Text)
}