package image_service

import (
	"bytes"
	"encoding/json"
	"feed/internal/lib/api/image"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type ImageService struct {
	Log          *slog.Logger
	BaseURL      string
	GetImagesURL string
	HTTPClient   *http.Client
}

func New(log *slog.Logger, baseURL, getImagesURL string, timeout time.Duration) *ImageService {
	return &ImageService{
		Log:          log,
		BaseURL:      baseURL,
		GetImagesURL: getImagesURL,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (i *ImageService) GetImages(imageIDs []string) ([]image.ImageResponse, error) {
	const op = "services.image_service.GetImages"

	if len(imageIDs) == 0 {
		return []image.ImageResponse{}, nil
	}

	reqBody := map[string][]string{"image_ids": imageIDs}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to marshal request: %w", op, err)
	}

	req, err := http.NewRequest("POST", i.BaseURL+i.GetImagesURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("%s: failed to create request: %w", op, err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := i.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to send request: %w", op, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to read response: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK {
		i.Log.Warn("image service returned error",
			slog.String("op", op),
			slog.Int("status_code", resp.StatusCode),
			slog.String("response", string(body)))
		return nil, fmt.Errorf("%s: image service returned status %d", op, resp.StatusCode)
	}

	var imagesResp struct {
		Images []image.ImageResponse `json:"images"`
	}
	if err := json.Unmarshal(body, &imagesResp); err != nil {
		return nil, fmt.Errorf("%s: failed to unmarshal response: %w", op, err)
	}

	return imagesResp.Images, nil
}
