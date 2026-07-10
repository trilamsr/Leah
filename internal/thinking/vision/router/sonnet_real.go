package router

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/trilam/leah/internal/keychain"
	"github.com/trilam/leah/internal/vision"
)

const sonnetVisionModel = "claude-sonnet-4-6"

// AnthropicSonnetClient is the production SonnetClient — base64-encodes the
// frame as PNG and streams a Messages call with an Image + Text content pair.
// Sits next to reasoner.AnthropicClient (mirrors the same SDK + env key) but
// stays in package router so router tests don't drag the LLM-dim metrics in.
type AnthropicSonnetClient struct {
	sdk   anthropic.Client
	model string
}

// NewSonnetClient builds the production client. Reads ANTHROPIC_API_KEY +
// LEAH_MODEL identically to reasoner.NewAnthropicClient — operator pins one
// model env-wide, not per-leg.
func NewSonnetClient() (*AnthropicSonnetClient, error) {
	key, err := keychain.LoadAnthropicKey()
	if err != nil {
		return nil, fmt.Errorf("load anthropic key: %w", err)
	}
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set (env or Keychain slot %s/%s)", keychain.AnthropicService, keychain.DefaultAccount)
	}
	model := sonnetVisionModel
	if v := os.Getenv("LEAH_MODEL"); v != "" {
		model = v
	}
	return &AnthropicSonnetClient{
		sdk:   anthropic.NewClient(option.WithAPIKey(key)),
		model: model,
	}, nil
}

// StreamVision encodes frame as PNG, sends Image+Text to the Anthropic
// Messages streaming endpoint, and returns text deltas on a buffered channel.
// Closes the channel on stream end, ctx cancel, or terminal error — caller's
// for-range loop terminates without leaking the SDK SSE goroutine.
func (c *AnthropicSonnetClient) StreamVision(ctx context.Context, frame vision.Image, prompt string) (<-chan VisionChunk, error) {
	_, encoded, err := encodeFrameForSonnet(frame)
	if err != nil {
		return nil, err
	}
	b64 := base64.StdEncoding.EncodeToString(encoded)
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64("image/png", b64),
				anthropic.NewTextBlock(prompt),
			),
		},
	}
	stream := c.sdk.Messages.NewStreaming(ctx, params)
	out := make(chan VisionChunk, 16)
	go func() {
		defer func() { _ = stream.Close() }()
		defer close(out)
		for stream.Next() {
			ev := stream.Current()
			if v, ok := ev.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
				if td := v.Delta.AsTextDelta(); td.Text != "" {
					select {
					case <-ctx.Done():
						return
					case out <- VisionChunk{Text: td.Text}:
					}
				}
			}
		}
		// Surface mid-stream SDK errors — Next() returns false on both EOF and
		// failure (timeout, rate-limit, auth, invalid-image). Without this
		// check the router would treat a network drop as a clean turn-end and
		// the HUD would render a silently-truncated answer.
		if err := stream.Err(); err != nil {
			select {
			case <-ctx.Done():
			case out <- VisionChunk{Err: err}:
			}
		}
	}()
	return out, nil
}

// encodeFrameForSonnet converts a vision.Image (raw RGBA / Gray pixels) into
// PNG bytes. Anthropic Messages accepts PNG/JPEG/GIF/WebP base64 sources;
// PNG is lossless and the encoder ships with the standard library — no
// external dep, no quality knob to mistune. Returns ("image/png", bytes).
func encodeFrameForSonnet(frame vision.Image) (string, []byte, error) {
	bpp, err := vision.BytesPerPixel(frame.MIME)
	if err != nil {
		return "", nil, err
	}
	if want := frame.Width * frame.Height * bpp; len(frame.Pixels) < want {
		return "", nil, fmt.Errorf("vision: pixel buffer too small for %dx%d@%dbpp (have %d, want %d)",
			frame.Width, frame.Height, bpp, len(frame.Pixels), want)
	}
	var img image.Image
	switch bpp {
	case 4:
		rgba := image.NewRGBA(image.Rect(0, 0, frame.Width, frame.Height))
		copy(rgba.Pix, frame.Pixels[:frame.Width*frame.Height*4])
		img = rgba
	case 1:
		gray := image.NewGray(image.Rect(0, 0, frame.Width, frame.Height))
		copy(gray.Pix, frame.Pixels[:frame.Width*frame.Height])
		img = gray
	default:
		return "", nil, fmt.Errorf("vision: unsupported bpp %d", bpp)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", nil, fmt.Errorf("vision: png encode: %w", err)
	}
	return "image/png", buf.Bytes(), nil
}
