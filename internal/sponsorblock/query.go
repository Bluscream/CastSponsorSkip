package sponsorblock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"

	"gabe565.com/castsponsorskip/internal/config"
)

type Video struct {
	VideoID  string    `json:"videoID"`
	Segments []Segment `json:"segments"`
}

type Segment struct {
	Segment       [2]float32 `json:"segment"`
	UUID          string     `json:"UUID"`
	Category      string     `json:"category"`
	VideoDuration float32    `json:"videoDuration"`
	ActionType    string     `json:"actionType"`
	Locked        int        `json:"locked"`
	Votes         int        `json:"votes"`
	Description   string     `json:"description"`
}

var ErrStatusCode = errors.New("invalid response status")

//nolint:gochecknoglobals
var baseURL = url.URL{
	Scheme: "https",
	Host:   "sponsor.ajay.app",
}

func QuerySegments(ctx context.Context, conf *config.Config, id string) ([]Segment, error) {
	checksumBytes := sha256.Sum256([]byte(id))
	checksum := hex.EncodeToString(checksumBytes[:])

	query := make(url.Values, len(conf.Categories)+len(conf.ActionTypes))
	for _, category := range conf.Categories {
		query.Add("category", category)
	}
	for _, actionType := range conf.ActionTypes {
		query.Add("actionType", actionType)
	}

	u := baseURL
	u.Path = path.Join("api", "skipSegments", checksum[:4])
	u.RawQuery = query.Encode()

	slog.Debug("Request segments", "url", u.String())
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: %s %s", ErrStatusCode, resp.Status, body)
	}

	var videos []Video
	if err := json.NewDecoder(resp.Body).Decode(&videos); err != nil {
		return nil, err
	}

	for _, video := range videos {
		if video.VideoID == id {
			return video.Segments, nil
		}
	}

	return nil, nil
}
