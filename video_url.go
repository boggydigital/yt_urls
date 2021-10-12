package yt_urls

import (
	"net/url"
)

const (
	videoParam  = "v"
	httpsScheme = "https"
	youtubeHost = "www.youtube.com"
	watchPath   = "watch"
)

//VideoUrl provides a URL for a video-id,
//e.g. http://www.youtube.com/watch?v=video-id1 for "video-id1"
func VideoUrl(videoId string) *url.URL {
	watchUrl := &url.URL{
		Scheme: httpsScheme,
		Host:   youtubeHost,
		Path:   watchPath,
	}

	q := watchUrl.Query()
	q.Add(videoParam, videoId)
	watchUrl.RawQuery = q.Encode()

	return watchUrl
}

//VideoId extracts video-id from a VideoUrl conforming URL
func VideoId(ytUrlStr string) (string, error) {
	ytUrl, err := url.Parse(ytUrlStr)
	if err != nil {
		return ytUrlStr, err
	}

	q := ytUrl.Query()
	if q.Has(videoParam) {
		return q.Get(videoParam), nil
	} else {
		return ytUrlStr, nil
	}
}
