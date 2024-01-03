package yt_urls

import (
	"bytes"
	"encoding/json"
	"golang.org/x/net/html"
	"net/http"
	"strings"
)

const (
	ytInitialData = "var ytInitialData"
)

type initialDataScriptMatcher struct{}

// initialDataScript is an HTML node filter for YouTube <script> text content
// that contains ytInitialData
func (idsm *initialDataScriptMatcher) Match(node *html.Node) bool {
	if node.Type != html.TextNode ||
		node.Parent == nil ||
		node.Parent.Data != "script" {
		return false
	}

	return strings.HasPrefix(node.Data, ytInitialData)
}

type PlaylistHeaderRenderer struct {
	PlaylistId string `json:"playlistId"`
	Title      struct {
		SimpleText string `json:"simpleText"`
	} `json:"title"`
	PlaylistHeaderBanner struct {
		HeroPlaylistThumbnailRenderer struct {
			Thumbnail struct {
				Thumbnails []Thumbnail `json:"thumbnails"`
			} `json:"thumbnail"`
		} `json:"heroPlaylistThumbnailRenderer"`
	} `json:"playlistHeaderBanner"`
	DescriptionText SimpleText `json:"descriptionText"`
	OwnerText       struct {
		Runs []OwnerTextRun `json:"runs"`
	} `json:"ownerText"`
	ViewCountText SimpleText `json:"viewCountText"`
	Privacy       string     `json:"privacy"`
}

// PlaylistInitialData is a minimal set of data structures required to decode and
// extract videoIds for playlist URL ytInitialData
type PlaylistInitialData struct {
	Contents struct {
		TwoColumnBrowseResultsRenderer struct {
			Tabs []struct {
				TabRenderer struct {
					Content struct {
						SectionListRenderer struct {
							Contents []struct {
								ItemSectionRenderer struct {
									Contents []struct {
										PlaylistVideoListRenderer struct {
											Contents   []PlaylistVideoListRendererContent `json:"contents"`
											PlaylistId string                             `json:"playlistId"`
										} `json:"playlistVideoListRenderer"`
									} `json:"contents"`
								} `json:"itemSectionRenderer"`
							} `json:"contents"`
						} `json:"sectionListRenderer"`
					} `json:"content"`
				} `json:"tabRenderer"`
			} `json:"tabs"`
		} `json:"twoColumnBrowseResultsRenderer"`
	} `json:"contents"`
	Header struct {
		PlaylistHeaderRenderer PlaylistHeaderRenderer `json:"playlistHeaderRenderer"`
	} `json:"header"`

	videoListContent []PlaylistVideoListRendererContent
	Context          *ytCfgInnerTubeContext
}

type OwnerTextRun struct {
	Text               string             `json:"text"`
	NavigationEndpoint NavigationEndpoint `json:"navigationEndpoint"`
}

type PlaylistVideoListRendererContent struct {
	PlaylistVideoRenderer    PlaylistVideoRenderer
	ContinuationItemRenderer ContinuationItemRenderer
}

type PlaylistVideoRenderer struct {
	VideoId string `json:"videoId"`
	Title   struct {
		Runs []struct {
			Text string `json:"text"`
		} `json:"runs"`
	} `json:"title"`
	// normally contains video channel title
	ShortBylineText struct {
		Runs []struct {
			Text string `json:"text"`
		} `json:"runs"`
	} `json:"shortBylineText"`
}

type ContinuationEndpoint struct {
	CommandMetadata struct {
		WebCommandMetadata struct {
			SendPost bool   `json:"sendPost"`
			ApiUrl   string `json:"apiUrl"`
		} `json:"webCommandMetadata"`
	} `json:"commandMetadata"`
	ContinuationCommand struct {
		Token   string `json:"token"`
		Request string `json:"request"`
	} `json:"continuationCommand"`
}

type ContinuationItemRenderer struct {
	Trigger              string               `json:"trigger"`
	ContinuationEndpoint ContinuationEndpoint `json:"continuationEndpoint"`
}

type VideoIdTitleChannel struct {
	VideoId string
	Title   string
	Channel string
}

func (id *PlaylistInitialData) PlaylistHeader() PlaylistHeaderRenderer {
	return id.Header.PlaylistHeaderRenderer
}

func (id *PlaylistInitialData) PlaylistContent() []PlaylistVideoListRendererContent {

	if id.videoListContent == nil {
		pvlc := make([]PlaylistVideoListRendererContent, 0)

		for _, tab := range id.Contents.TwoColumnBrowseResultsRenderer.Tabs {
			for _, sectionList := range tab.TabRenderer.Content.SectionListRenderer.Contents {
				for _, itemSection := range sectionList.ItemSectionRenderer.Contents {
					pvlc = append(pvlc, itemSection.PlaylistVideoListRenderer.Contents...)
				}
			}
		}

		id.videoListContent = pvlc
	}

	return id.videoListContent
}

func (id *PlaylistInitialData) SetContent(ct []PlaylistVideoListRendererContent) {
	id.videoListContent = ct
}

func (id *PlaylistInitialData) PlaylistOwner() string {
	ownerTextRuns := make([]string, 0, len(id.Header.PlaylistHeaderRenderer.OwnerText.Runs))
	for _, r := range id.Header.PlaylistHeaderRenderer.OwnerText.Runs {
		ownerTextRuns = append(ownerTextRuns, r.Text)
	}
	return strings.Join(ownerTextRuns, "")
}

func (pid *PlaylistInitialData) Videos() []VideoIdTitleChannel {
	var vits []VideoIdTitleChannel
	pc := pid.PlaylistContent()
	vits = make([]VideoIdTitleChannel, 0, len(pc))
	for _, vlc := range pc {
		videoId := vlc.PlaylistVideoRenderer.VideoId
		if videoId == "" {
			continue
		}
		title, titleRuns := "", vlc.PlaylistVideoRenderer.Title.Runs
		for _, r := range titleRuns {
			title += r.Text
		}
		sbTitle, sbTitleRuns := "", vlc.PlaylistVideoRenderer.ShortBylineText.Runs
		for _, r := range sbTitleRuns {
			sbTitle += r.Text
		}
		vits = append(vits, VideoIdTitleChannel{
			VideoId: videoId,
			Title:   title,
			Channel: sbTitle,
		})
	}
	return vits
}

func (pid *PlaylistInitialData) HasContinuation() bool {
	pc := pid.PlaylistContent()
	for i := len(pc) - 1; i >= 0; i-- {
		if pc[i].ContinuationItemRenderer.Trigger != "" {
			return true
		}
	}
	return false
}

func (pid *PlaylistInitialData) continuationEndpoint() *ContinuationEndpoint {
	pc := pid.PlaylistContent()
	for i := len(pc) - 1; i >= 0; i-- {
		if pc[i].ContinuationItemRenderer.Trigger != "" {
			return &pc[i].ContinuationItemRenderer.ContinuationEndpoint
		}
	}
	return nil
}

func (pid *PlaylistInitialData) Continue(client *http.Client) error {

	if !pid.HasContinuation() {
		return nil
	}

	contEndpoint := pid.continuationEndpoint()

	data := browseRequest{
		Context:      pid.Context.InnerTubeContext,
		Continuation: contEndpoint.ContinuationCommand.Token,
	}

	b := new(bytes.Buffer)
	if err := json.NewEncoder(b).Encode(data); err != nil {
		return err
	}

	browseUrl := BrowseUrl(
		contEndpoint.CommandMetadata.WebCommandMetadata.ApiUrl,
		pid.Context.APIKey)

	resp, err := client.Post(browseUrl.String(), contentType, b)
	defer resp.Body.Close()

	if err != nil {
		return err
	}

	var br browseResponse
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		return err
	}

	// update contents internals
	pid.SetContent(br.OnResponseReceivedActions[0].AppendContinuationItemsAction.ContinuationItems)

	return nil
}