package youtube

import (
	"encoding/json"
	"testing"
	"time"
)

const collabLockup = `{"contentType":"LOCKUP_CONTENT_TYPE_VIDEO","contentId":"abcdefghijk",
"metadata":{"lockupMetadataViewModel":{"title":{"content":"MARCANT"},
"image":{"avatarStackViewModel":{"avatars":[{"avatarViewModel":{"image":{"sources":[{"url":"https://yt3/stack.jpg"}]}}}]}},
"metadata":{"contentMetadataViewModel":{"metadataRows":[
{"metadataParts":[{"text":{"content":"I-Vincents Zweiter ne-eliasundmicka","commandRuns":[{"onTap":{"innertubeCommand":
{"showDialogCommand":{"panelLoadingStrategy":{"inlineContent":{"dialogViewModel":{"customContent":{"listViewModel":{"listItems":[
{"listItemViewModel":{"title":{"content":"Vincents Zweiter","commandRuns":[{"onTap":{"innertubeCommand":
{"browseEndpoint":{"browseId":"UCvincentszweiter00000000"},"commandMetadata":{"webCommandMetadata":{"url":"/@vincentszweiter"}}}}}]},
"leadingAccessory":{"avatarViewModel":{"image":{"sources":[{"url":"//yt3/vincent.jpg"}]}}}}},
{"listItemViewModel":{"title":{"content":"eliasundmicka"}}}]}}}}}}}}}]}}]},
{"metadataParts":[{"text":{"content":"201 ukubukwa"}}]}]}}}}}`

func TestCollabLockupTakesTheFirstCollaborator(t *testing.T) {
	var l map[string]any
	if err := json.Unmarshal([]byte(collabLockup), &l); err != nil {
		t.Fatal(err)
	}
	it, ok := FromLockup(l, time.Now())
	if !ok {
		t.Fatal("lockup not read")
	}
	if it.UploaderName != "Vincents Zweiter" || it.UploaderURL != ChannelURL("UCvincentszweiter00000000") ||
		it.UploaderAvatarURL != "https://yt3/vincent.jpg" {
		t.Fatalf("uploader %q %q %q", it.UploaderName, it.UploaderURL, it.UploaderAvatarURL)
	}
	if it.ViewCount != 201 {
		t.Fatalf("views %d", it.ViewCount)
	}
}

const collabVideoRenderer = `{"videoId":"abcdefghijk","title":{"runs":[{"text":"MARCANT"}]},
"ownerText":{"runs":[{"text":"I-Vincents Zweiter ne-eliasundmicka","navigationEndpoint":{"showDialogCommand":{"panelLoadingStrategy":
{"inlineContent":{"dialogViewModel":{"customContent":{"listViewModel":{"listItems":[
{"listItemViewModel":{"title":{"content":"Vincents Zweiter"},"leadingAccessory":{"avatarViewModel":{"image":{"sources":[{"url":"https://yt3/vincent.jpg"}]}}},
"rendererContext":{"commandContext":{"onTap":{"innertubeCommand":{"browseEndpoint":{"browseId":"UCvincentszweiter00000000"},
"commandMetadata":{"webCommandMetadata":{"url":"/channel/UCvincentszweiter00000000"}}}}}}}},
{"listItemViewModel":{"title":{"content":"eliasundmicka"}}}]}}}}}}}}]},
"channelThumbnailSupportedRenderers":{"channelThumbnailWithLinkRenderer":{"thumbnail":{"thumbnails":[{"url":"https://yt3/stack.jpg"}]}}}}`

func TestCollabSearchResultTakesTheFirstCollaborator(t *testing.T) {
	var v map[string]any
	if err := json.Unmarshal([]byte(collabVideoRenderer), &v); err != nil {
		t.Fatal(err)
	}
	it, ok := FromVideoRenderer(v, time.Now())
	if !ok || it.UploaderName != "Vincents Zweiter" || it.UploaderURL != ChannelURL("UCvincentszweiter00000000") ||
		it.UploaderAvatarURL != "https://yt3/vincent.jpg" {
		t.Fatalf("uploader %q %q %q", it.UploaderName, it.UploaderURL, it.UploaderAvatarURL)
	}
}

func TestCollabOnTheChannelTabTakesTheChannel(t *testing.T) {
	var l map[string]any
	if err := json.Unmarshal([]byte(`{"richItemRenderer":{"content":{"lockupViewModel":{"contentType":"LOCKUP_CONTENT_TYPE_VIDEO",
"contentId":"abcdefghijk","metadata":{"lockupMetadataViewModel":{"title":{"content":"PSYCHOLOGE"},"metadata":{"contentMetadataViewModel":
{"metadataRows":[{"metadataParts":[{"text":{"content":"I-eliasundmicka ne-eliasundmickaVOD"}}]},
{"metadataParts":[{"text":{"content":"2.6K ukubukwa"}}]}]}}}}}}}}`), &l); err != nil {
		t.Fatal(err)
	}
	ch := &Channel{ID: "UCeliasundmicka000000000", Name: "eliasundmicka", AvatarURL: "https://yt3/elias.jpg"}
	(&Client{}).fillGrid(ch, []any{l})
	if len(ch.Videos) != 1 {
		t.Fatalf("videos %d", len(ch.Videos))
	}
	v := ch.Videos[0]
	if v.UploaderName != "eliasundmicka" || v.UploaderURL != ChannelURL("UCeliasundmicka000000000") || v.UploaderAvatarURL != "https://yt3/elias.jpg" {
		t.Fatalf("uploader %q %q %q", v.UploaderName, v.UploaderURL, v.UploaderAvatarURL)
	}
}
