// Package imagegen implements the ImageGenerator interface against OpenAI's
// gpt-image-1.5 model, persisting the result via blobstore.
//
// The cover image is per-Issue (one only) per ADR-0006; this package does not
// concern itself with image policy beyond producing one ImageRef per call.
package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/thebitmonk/ai_newsletter/internal/blobstore"
)

const DefaultModel = "gpt-image-1.5"

// PromptHints carries the data the cover-image prompt is built from.
type PromptHints struct {
	IssueTitle   string
	StorySummaries []string
}

// ImageRef points at a stored image.
type ImageRef struct {
	URL      string
	MIMEType string
}

// ImageGenerator produces one cover image per call.
type ImageGenerator interface {
	Generate(ctx context.Context, brief string, hints PromptHints) (ImageRef, error)
}

// Client implements ImageGenerator using OpenAI + blobstore.
type Client struct {
	openai   openai.Client
	model    string
	blobs    blobstore.BlobStore
	keyPrefix string
}

// NewFromEnv loads OPENAI_API_KEY (required) and OPENAI_IMAGE_MODEL
// (optional, default gpt-image-1.5) and the supplied blobstore.
func NewFromEnv(blobs blobstore.BlobStore) (*Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("imagegen: OPENAI_API_KEY is required")
	}
	model := os.Getenv("OPENAI_IMAGE_MODEL")
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		openai:    openai.NewClient(option.WithAPIKey(apiKey)),
		model:     model,
		blobs:     blobs,
		keyPrefix: "issues/covers",
	}, nil
}

// New constructs a Client with explicit args.
func New(apiKey, model string, blobs blobstore.BlobStore) *Client {
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		openai:    openai.NewClient(option.WithAPIKey(apiKey)),
		model:     model,
		blobs:     blobs,
		keyPrefix: "issues/covers",
	}
}

// Generate produces a cover image, uploads it to the blobstore, and returns
// its public URL.
//
// Note on response_format: only DALL-E models accept this parameter; the
// newer gpt-image-1 family rejects it (400 unknown_parameter) and always
// returns b64_json. We omit response_format for gpt-image-* and set it for
// dall-e-*.
func (c *Client) Generate(ctx context.Context, brief string, hints PromptHints) (ImageRef, error) {
	prompt := buildPrompt(brief, hints)

	params := openai.ImageGenerateParams{
		Model:  openai.ImageModel(c.model),
		Prompt: prompt,
		N:      openai.Int(1),
		Size:   openai.ImageGenerateParamsSize1024x1024,
	}
	if strings.HasPrefix(c.model, "dall-e-") {
		params.ResponseFormat = openai.ImageGenerateParamsResponseFormatB64JSON
	}

	resp, err := c.openai.Images.Generate(ctx, params)
	if err != nil {
		return ImageRef{}, fmt.Errorf("imagegen: openai: %w", err)
	}
	if len(resp.Data) == 0 {
		return ImageRef{}, errors.New("imagegen: empty response")
	}
	// gpt-image-* always returns b64; dall-e-* returns b64 only because we
	// asked above. URL-returning paths intentionally unsupported (we want the
	// bytes so we can persist them to R2 with our own key).
	b64 := resp.Data[0].B64JSON
	if b64 == "" {
		return ImageRef{}, errors.New("imagegen: response missing b64_json")
	}
	imgBytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ImageRef{}, fmt.Errorf("imagegen: decode b64: %w", err)
	}

	key := fmt.Sprintf("%s/%s.png", c.keyPrefix, uuid.New())
	obj, err := c.blobs.Put(ctx, key, bytes.NewReader(imgBytes), int64(len(imgBytes)), "image/png")
	if err != nil {
		return ImageRef{}, fmt.Errorf("imagegen: blob put: %w", err)
	}
	return ImageRef{URL: obj.URL, MIMEType: obj.MIMEType}, nil
}

func buildPrompt(brief string, hints PromptHints) string {
	bullets := ""
	for _, s := range hints.StorySummaries {
		bullets += "- " + s + "\n"
	}
	return fmt.Sprintf(`Create a wide cover image for a newsletter issue.

PUBLICATION BRIEF:
%s

ISSUE TITLE: %s

ISSUE STORY HEADLINES:
%s

Style: editorial, magazine-quality, clean composition. No text overlays.
Avoid: stock-photo cliches, clipart, watermarks.`,
		brief, hints.IssueTitle, bullets)
}
