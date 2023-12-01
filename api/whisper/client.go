package whisper

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/akhilsharma90/go-whisper-project/models"
	"github.com/akhilsharma90/go-whisper-project/transcribe"
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

func (c *Client) TranscribeFile(file string, opts ...transcribe.TranscribeOption) (*models.TranscribeResponse, error) {
	h, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer h.Close()

	opts = append([]transcribe.TranscribeOption{transcribe.WithFile(filepath.Base(file))}, opts...)
	return c.Transcribe(h, opts...)
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


// Transcribe transcribes the given audio stream using the Whisper ASR API.
func (c *Client) Transcribe(h io.Reader, opts ...transcribe.TranscribeOption) (*models.TranscribeResponse, error) {
	if c.apiKey == "" {
		return nil, errors.New("missing API key (set OPENAI_API_KEY in env)")
	}

	tc := &transcribe.TranscribeConfig{}
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

	var tr models.TranscribeResponse
	if err = json.NewDecoder(r).Decode(&tr); err != nil {
		return nil, err
	}
	return &tr, nil
}
