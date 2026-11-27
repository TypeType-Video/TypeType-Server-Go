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
