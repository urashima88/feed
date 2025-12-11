package tag_service

import (
	"bytes"
	"encoding/json"
	"feed/internal/lib/api/tag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type TagService struct {
	Log           *slog.Logger
	BaseURL       string
	CreateTagsURL string
	GetTagsURL    string
	HTTPClient    *http.Client
}

type Request struct {
	Tags []string `json:"tags"`
}

type Response struct {
	Tags []tag.Tag `json:"tags"`
}

func New(log *slog.Logger, baseURL, createTagsURL, getTagsURL string, timeout time.Duration) *TagService {
	return &TagService{
		Log:           log,
		BaseURL:       baseURL,
		CreateTagsURL: createTagsURL,
		GetTagsURL:    getTagsURL,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (i *TagService) CreateTags(tags []string) ([]tag.Tag, error) {
	const op = "services.tag_service.CreateTags"

	if len(tags) == 0 {
		return []tag.Tag{}, nil
	}

	reqBody := Request{Tags: tags}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to marshal request: %w", op, err)
	}

	req, err := http.NewRequest("POST", i.BaseURL+i.CreateTagsURL, bytes.NewBuffer(jsonData))
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

	var tagsResp Response
	parseErr := json.Unmarshal(body, &tagsResp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || parseErr != nil {

		i.Log.Warn("tag service returned error for tags",
			slog.String("op", op),
			slog.Int("status_code", resp.StatusCode),
			slog.String("response", string(body)),
			slog.String("parse_error", func() string {
				if parseErr != nil {
					return parseErr.Error()
				}
				return ""
			}()))

		if parseErr == nil && len(tagsResp.Tags) > 0 {
			return tagsResp.Tags, fmt.Errorf("%s: tag service returned status %d, got only %d tags", op, resp.StatusCode, len(tagsResp.Tags))
		}
		return nil, fmt.Errorf("%s: tag service error: status %d", op, resp.StatusCode)
	}
	return tagsResp.Tags, nil
}

func (i *TagService) GetTags(tagIDs []string) ([]tag.Tag, error) {
	const op = "services.tag_service.GetTags"

	if len(tagIDs) == 0 {
		return []tag.Tag{}, nil
	}

	reqBody := map[string][]string{"tag_ids": tagIDs}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to marshal request: %w", op, err)
	}

	req, err := http.NewRequest("POST", i.BaseURL+i.GetTagsURL, bytes.NewBuffer(jsonData))
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
		i.Log.Warn("tag service returned error for tags",
			slog.String("op", op),
			slog.Int("status_code", resp.StatusCode),
			slog.String("response", string(body)))

		return nil, fmt.Errorf("%s: tag service returned status: %d", op, resp.StatusCode)
	}

	var tagsResp struct {
		Tags []tag.Tag `json:"tags"`
	}
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, fmt.Errorf("%s: failed to unmarshal response: %w", op, err)
	}

	return tagsResp.Tags, nil
}
